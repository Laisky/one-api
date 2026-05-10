package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/model"
)

// Client defines MCP operations required by the aggregator.
type Client interface {
	ListTools(ctx context.Context) ([]ToolDescriptor, error)
	CallTool(ctx context.Context, name string, arguments any) (*CallToolResult, error)
}

// StreamableHTTPClient implements MCP client calls over the Streamable HTTP
// transport. The client performs the protocol handshake (initialize +
// notifications/initialized) lazily on first use, captures any
// server-issued Mcp-Session-Id, and supports both JSON and SSE response
// content types.
type StreamableHTTPClient struct {
	BaseURL string
	Headers map[string]string
	Timeout time.Duration
	Logger  glog.Logger

	initMu          sync.Mutex
	initialized     bool
	sessionID       string
	protocolVersion string
}

const (
	mcpProtocolVersionHeader  = "Mcp-Protocol-Version"
	mcpSessionIDHeader        = "Mcp-Session-Id"
	mcpDefaultProtocolVersion = "2025-06-18"
	mcpAcceptHeaderValue      = "application/json, text/event-stream"
	mcpClientName             = "one-api-mcp-client"
	mcpClientVersion          = "1.0.0"
)

// NewStreamableHTTPClient constructs a StreamableHTTPClient from MCP server metadata.
func NewStreamableHTTPClient(server *model.MCPServer, headers map[string]string, timeout time.Duration) *StreamableHTTPClient {
	return newStreamableHTTPClient(server, headers, timeout, nil)
}

// NewStreamableHTTPClientWithLogger constructs a StreamableHTTPClient with logging enabled.
func NewStreamableHTTPClientWithLogger(server *model.MCPServer, headers map[string]string, timeout time.Duration, logger glog.Logger) *StreamableHTTPClient {
	return newStreamableHTTPClient(server, headers, timeout, logger)
}

// newStreamableHTTPClient constructs a StreamableHTTPClient from MCP server metadata.
// The Mcp-Session-Id header is intentionally NOT pre-populated — per the
// Streamable HTTP transport spec, the session id is issued by the server in
// the initialize response and only then attached to subsequent requests.
func newStreamableHTTPClient(server *model.MCPServer, headers map[string]string, timeout time.Duration, logger glog.Logger) *StreamableHTTPClient {
	merged := make(map[string]string)
	for k, v := range server.Headers {
		merged[k] = v
	}
	for k, v := range headers {
		merged[k] = v
	}
	if _, ok := merged[mcpProtocolVersionHeader]; !ok {
		merged[mcpProtocolVersionHeader] = mcpDefaultProtocolVersion
	}
	if _, ok := merged["Accept"]; !ok {
		merged["Accept"] = mcpAcceptHeaderValue
	}

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

	return &StreamableHTTPClient{
		BaseURL: strings.TrimSpace(server.BaseURL),
		Headers: merged,
		Timeout: timeout,
		Logger:  logger,
	}
}

// Initialize performs the MCP protocol handshake: sends an `initialize`
// request and the corresponding `notifications/initialized` notification.
// Captures the server-issued Mcp-Session-Id (if any) and the negotiated
// protocol version, then attaches both to subsequent requests.
//
// Safe to call concurrently and idempotent — the handshake runs at most
// once per client instance.
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

	respHeaders, err := c.doRPCRaw(ctx, "initialize", initParams, &initResult)
	if err != nil {
		return errors.Wrap(err, "mcp initialize")
	}

	if sid := respHeaders.Get(mcpSessionIDHeader); sid != "" {
		c.sessionID = sid
		c.Headers[mcpSessionIDHeader] = sid
	}
	if initResult.ProtocolVersion != "" {
		c.protocolVersion = initResult.ProtocolVersion
		c.Headers[mcpProtocolVersionHeader] = initResult.ProtocolVersion
	}

	if err := c.sendNotification(ctx, "notifications/initialized", nil); err != nil {
		// Notification failure is non-fatal — log and proceed so a server
		// that diverges on this notification does not block tool calls.
		if c.Logger != nil {
			c.Logger.Warn("mcp notifications/initialized failed", zap.Error(err))
		}
	}

	c.initialized = true
	return nil
}

// ListTools calls the MCP tools/list method.
func (c *StreamableHTTPClient) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	if err := c.Initialize(ctx); err != nil {
		return nil, err
	}
	var result struct {
		Tools []ToolDescriptor `json:"tools"`
	}
	if err := c.doRPCWithSessionRetry(ctx, "tools/list", nil, &result); err != nil {
		return nil, errors.Wrap(err, "mcp rpc tools/list")
	}
	return result.Tools, nil
}

// CallTool invokes a MCP tool by name.
func (c *StreamableHTTPClient) CallTool(ctx context.Context, name string, arguments any) (*CallToolResult, error) {
	if err := c.Initialize(ctx); err != nil {
		return nil, err
	}
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	var result CallToolResult
	if err := c.doRPCWithSessionRetry(ctx, "tools/call", params, &result); err != nil {
		return nil, errors.Wrapf(err, "mcp rpc tools/call %s", name)
	}
	return &result, nil
}

// doRPCWithSessionRetry performs one JSON-RPC call and retries once after
// re-initialization when the upstream reports an invalid MCP session id.
func (c *StreamableHTTPClient) doRPCWithSessionRetry(ctx context.Context, method string, params any, out any) error {
	err := c.doRPC(ctx, method, params, out)
	if err == nil {
		return nil
	}
	if !isMCPInvalidSessionError(err) {
		return err
	}

	c.resetInitialization()
	if initErr := c.Initialize(ctx); initErr != nil {
		return errors.Wrap(initErr, "reinitialize mcp session")
	}
	return c.doRPC(ctx, method, params, out)
}

// doRPC performs a JSON-RPC call and discards the response headers.
func (c *StreamableHTTPClient) doRPC(ctx context.Context, method string, params any, out any) error {
	_, err := c.doRPCRaw(ctx, method, params, out)
	return err
}

// resetInitialization clears negotiated MCP session/protocol state so a fresh
// initialize handshake can run again.
func (c *StreamableHTTPClient) resetInitialization() {
	c.initMu.Lock()
	defer c.initMu.Unlock()
	c.initialized = false
	c.sessionID = ""
	delete(c.Headers, mcpSessionIDHeader)
}

// doRPCRaw performs a JSON-RPC call and returns the response headers, which
// the initialize handshake needs to read the Mcp-Session-Id assigned by the
// server. Handles both `application/json` and `text/event-stream` response
// content types per the Streamable HTTP transport spec.
func (c *StreamableHTTPClient) doRPCRaw(ctx context.Context, method string, params any, out any) (http.Header, error) {
	if c == nil {
		return nil, errors.New("mcp client is nil")
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      random.GetUUID(),
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal mcp request")
	}

	client := &http.Client{Timeout: c.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "create mcp request")
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	c.debugLogRequest(method, req.Header, data)

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "send mcp request")
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp.Header, errors.Wrap(readErr, "read mcp response body")
	}
	c.debugLogResponse(method, resp, body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Header, errors.Errorf("mcp request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/event-stream") {
		jsonBody, perr := parseSSEResponse(body)
		if perr != nil {
			return resp.Header, errors.Wrap(perr, "parse mcp sse response")
		}
		body = jsonBody
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  map[string]any  `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return resp.Header, errors.Wrap(err, "decode mcp response")
	}
	if envelope.Error != nil {
		return resp.Header, errors.Errorf("mcp error: %v", envelope.Error)
	}
	if out == nil {
		return resp.Header, nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return resp.Header, errors.Wrap(err, "unmarshal mcp result")
	}
	return resp.Header, nil
}

// sendNotification sends a JSON-RPC notification (no `id` field). Per the
// Streamable HTTP transport spec, the server replies with HTTP 202 and an
// empty body — there is no JSON-RPC envelope to parse.
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

	client := &http.Client{Timeout: c.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "create mcp notification request")
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	c.debugLogRequest(method, req.Header, data)

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "send mcp notification")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	c.debugLogResponse(method, resp, body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf("mcp notification failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// parseSSEResponse extracts the JSON payload from a Server-Sent Events body.
// The MCP Streamable HTTP transport allows a server to reply to a single
// request/response with one SSE event whose `data:` field contains the
// JSON-RPC envelope. Multi-line `data:` fields are concatenated with `\n`
// per the SSE spec.
func parseSSEResponse(body []byte) ([]byte, error) {
	var dataLines []string
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
func (c *StreamableHTTPClient) debugLogRequest(method string, headers http.Header, body []byte) {
	if c == nil || c.Logger == nil {
		return
	}
	sanitizedHeaders := sanitizeHeadersForLog(headers)
	sanitizedBody := sanitizeBodyForLog(body)
	c.Logger.Debug("mcp outbound request",
		zap.String("method", method),
		zap.String("url", c.BaseURL),
		zap.Any("headers", sanitizedHeaders),
		zap.Int("body_bytes", len(body)),
		zap.String("body", sanitizedBody),
	)
}

// debugLogResponse records sanitized inbound MCP response metadata and payload.
func (c *StreamableHTTPClient) debugLogResponse(method string, resp *http.Response, body []byte) {
	if c == nil || c.Logger == nil || resp == nil {
		return
	}
	sanitizedHeaders := sanitizeHeadersForLog(resp.Header)
	sanitizedBody := sanitizeBodyForLog(body)
	c.Logger.Debug("mcp inbound response",
		zap.String("method", method),
		zap.String("url", c.BaseURL),
		zap.Int("status_code", resp.StatusCode),
		zap.Any("headers", sanitizedHeaders),
		zap.Int("body_bytes", len(body)),
		zap.String("body", sanitizedBody),
	)
}

// sanitizeHeadersForLog redacts sensitive header values for logging.
func sanitizeHeadersForLog(headers http.Header) map[string]string {
	if headers == nil {
		return nil
	}
	sanitized := make(map[string]string, len(headers))
	for key, values := range headers {
		lower := strings.ToLower(strings.TrimSpace(key))
		if isSensitiveKey(lower) {
			sanitized[key] = "<redacted>"
			continue
		}
		sanitized[key] = strings.Join(values, ",")
	}
	return sanitized
}

// sanitizeBodyForLog returns a sanitized body string for logging.
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

// scrubJSONValue redacts sensitive or binary-like data from JSON values.
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
		for idx, inner := range typed {
			typed[idx] = scrubJSONValue(inner, keyHint)
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

// isSensitiveKey reports whether a key is likely to contain secrets.
func isSensitiveKey(key string) bool {
	if key == "" {
		return false
	}
	sensitive := []string{"authorization", "proxy-authorization", "api_key", "apikey", "token", "secret", "password", "passwd", "x-api-key"}
	for _, token := range sensitive {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

// isBinaryKey reports whether a key is likely to contain binary payloads.
func isBinaryKey(key string) bool {
	if key == "" {
		return false
	}
	tokens := []string{"image", "audio", "video", "binary", "base64", "bytes", "file", "blob"}
	for _, token := range tokens {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

// isLikelyBinary performs a heuristic check for binary payloads.
func isLikelyBinary(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	if !utf8.Valid(body) {
		return true
	}
	nonPrintable := 0
	for _, r := range body {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			nonPrintable++
		}
	}
	return nonPrintable > len(body)/20
}

// isLikelyBase64 checks whether a string looks like base64 data.
func isLikelyBase64(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 128 {
		return false
	}
	if strings.HasPrefix(trimmed, "data:") {
		return true
	}
	for _, r := range trimmed {
		if r == '=' || r == '+' || r == '/' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			continue
		}
		return false
	}
	return true
}

// isMCPInvalidSessionError reports whether an RPC failure indicates that the
// server rejected or missed the current Mcp-Session-Id.
func isMCPInvalidSessionError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no valid session id provided")
}
