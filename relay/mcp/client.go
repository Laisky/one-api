package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/model"
)

// Client defines the MCP tool-listing and tool-call operations required by the aggregator.
type Client interface {
	ListTools(ctx context.Context) ([]ToolDescriptor, error)
	CallTool(ctx context.Context, name string, arguments any) (*CallToolResult, error)
}

// StreamableHTTPClient implements modern and legacy MCP over the Streamable HTTP transport.
type StreamableHTTPClient struct {
	BaseURL string
	Headers map[string]string
	Timeout time.Duration
	Logger  glog.Logger

	serverRef identity.MCPServerRef

	headerMu        sync.RWMutex
	initMu          sync.Mutex
	initialized     bool
	sessionID       string
	protocolVersion string
}

const (
	mcpProtocolVersionHeader  = ProtocolVersionHeader
	mcpSessionIDHeader        = SessionIDHeader
	mcpDefaultProtocolVersion = LegacyProtocolVersion
	mcpAcceptHeaderValue      = "application/json, text/event-stream"
	mcpClientName             = "one-api-mcp-client"
	mcpClientVersion          = "1.0.0"
)

// NewStreamableHTTPClient constructs a StreamableHTTPClient from MCP server metadata.
//
// Parameters:
//   - server: the configured upstream MCP server.
//   - headers: request headers that override server-level configured headers.
//   - timeout: the per-request HTTP timeout.
//
// Return values:
//   - *StreamableHTTPClient: a client configured for modern-first requests and legacy fallback.
func NewStreamableHTTPClient(server *model.MCPServer, headers map[string]string, timeout time.Duration) *StreamableHTTPClient {
	return newStreamableHTTPClient(server, headers, timeout, nil)
}

// NewStreamableHTTPClientWithLogger constructs a StreamableHTTPClient with request-aware logging.
//
// Parameters:
//   - server: the configured upstream MCP server.
//   - headers: request headers that override server-level configured headers.
//   - timeout: the per-request HTTP timeout.
//   - logger: the logger used for sanitized transport diagnostics.
//
// Return values:
//   - *StreamableHTTPClient: a client configured for modern-first requests and legacy fallback.
func NewStreamableHTTPClientWithLogger(server *model.MCPServer, headers map[string]string, timeout time.Duration, logger glog.Logger) *StreamableHTTPClient {
	return newStreamableHTTPClient(server, headers, timeout, logger)
}

// newStreamableHTTPClient merges configuration without preselecting a legacy protocol session.
//
// Parameters:
//   - server: the configured upstream MCP server.
//   - headers: request headers that override server-level configured headers.
//   - timeout: the per-request HTTP timeout.
//   - logger: the optional logger used for sanitized transport diagnostics.
//
// Return values:
//   - *StreamableHTTPClient: a new client whose legacy lifecycle is initialized lazily.
func newStreamableHTTPClient(server *model.MCPServer, headers map[string]string, timeout time.Duration, logger glog.Logger) *StreamableHTTPClient {
	merged := make(map[string]string)
	if server != nil {
		for key, value := range server.Headers {
			merged[key] = value
		}
	}
	for key, value := range headers {
		merged[key] = value
	}
	delete(merged, mcpProtocolVersionHeader)
	delete(merged, mcpSessionIDHeader)
	if _, exists := merged["Accept"]; !exists {
		merged["Accept"] = mcpAcceptHeaderValue
	}

	if server != nil {
		switch strings.ToLower(server.AuthType) {
		case model.MCPAuthTypeBearer:
			if server.APIKey != "" {
				merged["Authorization"] = "Bearer " + server.APIKey
			}
		case model.MCPAuthTypeAPIKey:
			if server.APIKey != "" {
				merged["X-API-Key"] = server.APIKey
			}
		}
	}

	client := &StreamableHTTPClient{
		Headers: merged,
		Timeout: timeout,
		Logger:  logger,
	}
	if server != nil {
		client.BaseURL = strings.TrimSpace(server.BaseURL)
		client.serverRef = server.Ref()
	}
	return client
}

// Initialize performs the preferred 2025-11-25 initialize exchange and records the negotiated legacy state.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//
// Return values:
//   - error: a wrapped transport, negotiation, or notification error when initialization cannot complete.
func (c *StreamableHTTPClient) Initialize(ctx context.Context) error {
	if c == nil {
		return errors.New("mcp client is nil")
	}
	c.initMu.Lock()
	defer c.initMu.Unlock()
	if c.initialized {
		return nil
	}

	initParams := map[string]any{
		"protocolVersion": mcpDefaultProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    mcpClientName,
			"version": mcpClientVersion,
		},
	}
	var initResult struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      map[string]any `json:"serverInfo"`
	}

	responseHeaders, err := c.doRPCRaw(ctx, "initialize", initParams, &initResult)
	if err != nil {
		return errors.Wrap(err, "mcp initialize")
	}

	negotiatedVersion := strings.TrimSpace(initResult.ProtocolVersion)
	if negotiatedVersion == "" {
		negotiatedVersion = mcpDefaultProtocolVersion
	}
	if !IsLegacyProtocolVersion(negotiatedVersion) {
		return errors.Errorf("mcp initialize negotiated unsupported legacy version %q", negotiatedVersion)
	}
	c.protocolVersion = negotiatedVersion
	c.setClientHeader(mcpProtocolVersionHeader, negotiatedVersion)
	if sessionID := strings.TrimSpace(responseHeaders.Get(mcpSessionIDHeader)); sessionID != "" {
		c.sessionID = sessionID
		c.setClientHeader(mcpSessionIDHeader, sessionID)
	}

	if err := c.sendNotification(ctx, "notifications/initialized", nil); err != nil {
		if c.Logger != nil {
			c.Logger.Warn("mcp notifications/initialized failed",
				append(c.serverRef.Zap(), zap.Error(err))...)
		}
	}
	c.initialized = true
	return nil
}

// ListTools lists every tool through the negotiated legacy lifecycle and follows pagination cursors.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//
// Return values:
//   - []ToolDescriptor: tools collected across every legacy tools/list page.
//   - error: a wrapped initialization, transport, pagination, or decoding error.
func (c *StreamableHTTPClient) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	if err := c.Initialize(ctx); err != nil {
		return nil, err
	}
	tools := make([]ToolDescriptor, 0)
	seenCursors := make(map[string]struct{})
	cursor := ""
	for page := 0; ; page++ {
		if page >= 1000 {
			return nil, errors.New("legacy mcp tools/list exceeded 1000 pages")
		}
		var params map[string]any
		if cursor != "" {
			params = map[string]any{"cursor": cursor}
		}
		var result ListToolsResult
		if err := c.doRPC(ctx, "tools/list", params, &result); err != nil {
			return nil, errors.Wrap(err, "mcp rpc tools/list")
		}
		tools = append(tools, result.Tools...)
		nextCursor := strings.TrimSpace(result.NextCursor)
		if nextCursor == "" {
			return tools, nil
		}
		if _, exists := seenCursors[nextCursor]; exists {
			return nil, errors.Errorf("legacy mcp tools/list repeated cursor %q", nextCursor)
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}
}

// CallTool invokes one exact case-sensitive tool through the negotiated legacy lifecycle.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - name: the exact upstream tool name.
//   - arguments: a JSON-compatible argument object; nil becomes an empty object.
//
// Return values:
//   - *CallToolResult: the decoded upstream result.
//   - error: a wrapped initialization, validation, transport, or decoding error.
func (c *StreamableHTTPClient) CallTool(ctx context.Context, name string, arguments any) (*CallToolResult, error) {
	if err := c.Initialize(ctx); err != nil {
		return nil, err
	}
	argumentMap, err := normalizeToolArguments(arguments)
	if err != nil {
		return nil, errors.Wrap(err, "normalize legacy mcp tool arguments")
	}
	params := map[string]any{
		"name":      name,
		"arguments": argumentMap,
	}
	var result CallToolResult
	if err := c.doRPC(ctx, "tools/call", params, &result); err != nil {
		return nil, errors.Wrapf(err, "mcp rpc tools/call %s", name)
	}
	return &result, nil
}

// doRPC performs one legacy JSON-RPC request and discards the response headers.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - method: the legacy JSON-RPC method.
//   - params: optional structured request parameters.
//   - out: the destination for a successful result, or nil to discard it.
//
// Return values:
//   - error: a wrapped transport, correlation, protocol, or decoding error.
func (c *StreamableHTTPClient) doRPC(ctx context.Context, method string, params any, out any) error {
	_, err := c.doRPCRaw(ctx, method, params, out)
	return err
}

// doRPCRaw performs one correlated legacy JSON-RPC request and returns the response headers.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - method: the legacy JSON-RPC method.
//   - params: optional structured request parameters.
//   - out: the destination for a successful result, or nil to discard it.
//
// Return values:
//   - http.Header: response headers used by initialize to capture session state.
//   - error: a wrapped transport, size, correlation, protocol, or decoding error.
func (c *StreamableHTTPClient) doRPCRaw(ctx context.Context, method string, params any, out any) (http.Header, error) {
	if c == nil {
		return nil, errors.New("mcp client is nil")
	}
	requestID := random.GetUUID()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal mcp request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "create mcp request")
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.headerSnapshot() {
		req.Header.Set(key, value)
	}
	c.debugLogRequest(method, req.Header, data)

	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "send mcp request")
	}
	defer resp.Body.Close()

	body, err := readMCPResponseBody(resp.Body)
	if err != nil {
		return resp.Header, errors.Wrap(err, "read mcp response body")
	}
	c.debugLogResponse(method, resp, body)

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		body, err = extractMCPResponseEnvelope(body, requestID)
		if err != nil {
			return resp.Header, errors.Wrap(err, "parse mcp SSE response")
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.Header, decodeModernProtocolError(resp.StatusCode, body)
	}

	envelope, err := parseMCPResponseEnvelope(body, requestID)
	if err != nil {
		return resp.Header, err
	}
	if envelope.Error != nil {
		return resp.Header, &ProtocolError{
			HTTPStatus: resp.StatusCode,
			Code:       envelope.Error.Code,
			Message:    envelope.Error.Message,
			Data:       envelope.Error.Data,
			Body:       strings.TrimSpace(string(body)),
		}
	}
	if out == nil {
		return resp.Header, nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return resp.Header, errors.Wrap(err, "unmarshal mcp result")
	}
	return resp.Header, nil
}

// sendNotification sends one legacy JSON-RPC notification without a request identifier.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - method: the notification method.
//   - params: optional structured notification parameters.
//
// Return values:
//   - error: a wrapped transport, size, or HTTP status error.
func (c *StreamableHTTPClient) sendNotification(ctx context.Context, method string, params any) error {
	if c == nil {
		return errors.New("mcp client is nil")
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		payload["params"] = params
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "marshal mcp notification")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "create mcp notification request")
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.headerSnapshot() {
		req.Header.Set(key, value)
	}
	c.debugLogRequest(method, req.Header, data)

	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "send mcp notification")
	}
	defer resp.Body.Close()
	body, err := readMCPResponseBody(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read mcp notification response body")
	}
	c.debugLogResponse(method, resp, body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return errors.Errorf("mcp notification failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// headerSnapshot returns a copy of mutable client headers for one HTTP request.
//
// Parameters: none.
//
// Return values:
//   - map[string]string: an isolated header map safe for request construction.
func (c *StreamableHTTPClient) headerSnapshot() map[string]string {
	if c == nil {
		return nil
	}
	c.headerMu.RLock()
	defer c.headerMu.RUnlock()
	snapshot := make(map[string]string, len(c.Headers))
	for key, value := range c.Headers {
		snapshot[key] = value
	}
	return snapshot
}

// setClientHeader updates one internally managed header under the client header lock.
//
// Parameters:
//   - key: the HTTP header name.
//   - value: the replacement value; an empty value removes the header.
//
// Return values: none.
func (c *StreamableHTTPClient) setClientHeader(key, value string) {
	if c == nil {
		return
	}
	c.headerMu.Lock()
	defer c.headerMu.Unlock()
	if value == "" {
		delete(c.Headers, key)
		return
	}
	c.Headers[key] = value
}

// parseSSEResponse extracts data fields from a single finite SSE message for compatibility tests.
//
// Parameters:
//   - body: a finite Server-Sent Events response body.
//
// Return values:
//   - []byte: concatenated data fields.
//   - error: an error when the body contains no data field.
func parseSSEResponse(body []byte) ([]byte, error) {
	dataLines := make([]string, 0)
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimRight(raw, "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
	}
	if len(dataLines) == 0 {
		return nil, errors.New("sse response has no data fields")
	}
	return []byte(strings.Join(dataLines, "\n")), nil
}

// debugLogRequest records sanitized outbound MCP request metadata and payload.
//
// Parameters:
//   - method: the JSON-RPC method.
//   - headers: the outbound HTTP headers.
//   - body: the encoded request body.
//
// Return values: none.
func (c *StreamableHTTPClient) debugLogRequest(method string, headers http.Header, body []byte) {
	if c == nil || c.Logger == nil {
		return
	}
	c.Logger.Debug("mcp outbound request",
		append(c.serverRef.Zap(),
			zap.String("method", method),
			zap.String("url", c.BaseURL),
			zap.Any("headers", sanitizeHeadersForLog(headers)),
			zap.Int("body_bytes", len(body)),
			zap.String("body", sanitizeBodyForLog(body)),
		)...)
}

// debugLogResponse records sanitized inbound MCP response metadata and payload.
//
// Parameters:
//   - method: the JSON-RPC method.
//   - resp: the HTTP response metadata.
//   - body: the bounded response body.
//
// Return values: none.
func (c *StreamableHTTPClient) debugLogResponse(method string, resp *http.Response, body []byte) {
	if c == nil || c.Logger == nil || resp == nil {
		return
	}
	c.Logger.Debug("mcp inbound response",
		append(c.serverRef.Zap(),
			zap.String("method", method),
			zap.String("url", c.BaseURL),
			zap.Int("status_code", resp.StatusCode),
			zap.Any("headers", sanitizeHeadersForLog(resp.Header)),
			zap.Int("body_bytes", len(body)),
			zap.String("body", sanitizeBodyForLog(body)),
		)...)
}

// sanitizeHeadersForLog redacts sensitive header values before structured logging.
//
// Parameters:
//   - headers: the HTTP headers to sanitize.
//
// Return values:
//   - map[string]string: flattened headers with sensitive values redacted.
func sanitizeHeadersForLog(headers http.Header) map[string]string {
	if headers == nil {
		return nil
	}
	sanitized := make(map[string]string, len(headers))
	for key, values := range headers {
		if isSensitiveKey(strings.ToLower(strings.TrimSpace(key))) {
			sanitized[key] = "<redacted>"
			continue
		}
		sanitized[key] = strings.Join(values, ",")
	}
	return sanitized
}

// sanitizeBodyForLog redacts secrets and binary-like fields from a request or response body.
//
// Parameters:
//   - body: the raw request or response body.
//
// Return values:
//   - string: a sanitized JSON string or a safe textual placeholder.
func sanitizeBodyForLog(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if isLikelyBinary(body) {
		return "<binary body omitted>"
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	if !json.Valid([]byte(trimmed)) {
		return trimmed
	}
	var payload any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return trimmed
	}
	payload = scrubJSONValue(payload, "")
	encoded, err := json.Marshal(payload)
	if err != nil {
		return trimmed
	}
	return string(encoded)
}

// scrubJSONValue recursively redacts sensitive and binary-like JSON values.
//
// Parameters:
//   - value: the decoded JSON value to sanitize.
//   - keyHint: the parent field name used for redaction heuristics.
//
// Return values:
//   - any: the sanitized JSON-compatible value.
func scrubJSONValue(value any, keyHint string) any {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, inner := range typed {
			lower := strings.ToLower(key)
			if isSensitiveKey(lower) {
				typed[key] = "<redacted>"
				continue
			}
			if isBinaryKey(lower) {
				typed[key] = "<binary omitted>"
				continue
			}
			typed[key] = scrubJSONValue(inner, lower)
		}
		return typed
	case []any:
		for index, inner := range typed {
			typed[index] = scrubJSONValue(inner, keyHint)
		}
		return typed
	case string:
		lowerKey := strings.ToLower(keyHint)
		if isSensitiveKey(lowerKey) {
			return "<redacted>"
		}
		if isBinaryKey(lowerKey) || isLikelyBase64(typed) || strings.HasPrefix(typed, "data:") {
			return "<binary omitted>"
		}
		return typed
	default:
		return value
	}
}

// isSensitiveKey reports whether a field or header name is likely to contain a secret.
//
// Parameters:
//   - key: a normalized field or header name.
//
// Return values:
//   - bool: true when the value must be redacted from logs.
func isSensitiveKey(key string) bool {
	if key == "" {
		return false
	}
	for _, token := range []string{"authorization", "proxy-authorization", "api_key", "apikey", "token", "secret", "password", "passwd", "x-api-key"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

// isBinaryKey reports whether a JSON field name is likely to contain binary payload data.
//
// Parameters:
//   - key: a normalized JSON field name.
//
// Return values:
//   - bool: true when the value should be omitted from logs.
func isBinaryKey(key string) bool {
	if key == "" {
		return false
	}
	for _, token := range []string{"image", "audio", "video", "binary", "base64", "bytes", "file", "blob"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

// isLikelyBinary reports whether a response body contains invalid UTF-8 or control-heavy data.
//
// Parameters:
//   - body: the raw body to inspect.
//
// Return values:
//   - bool: true when logging the body as text would be unsafe or unhelpful.
func isLikelyBinary(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if !utf8.Valid(body) {
		return true
	}
	nonPrintable := 0
	for _, value := range body {
		if value == '\n' || value == '\r' || value == '\t' {
			continue
		}
		if value < 0x20 || value == 0x7f {
			nonPrintable++
		}
	}
	return nonPrintable > len(body)/20
}

// isLikelyBase64 reports whether a long string resembles encoded binary data.
//
// Parameters:
//   - value: the string to inspect.
//
// Return values:
//   - bool: true when the value should be omitted from logs.
func isLikelyBase64(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 128 {
		return false
	}
	if strings.HasPrefix(trimmed, "data:") {
		return true
	}
	for _, character := range trimmed {
		if character == '=' || character == '+' || character == '/' || character == '-' || character == '_' || (character >= '0' && character <= '9') || (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') {
			continue
		}
		return false
	}
	return true
}
