package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/model"
)

// Client defines MCP operations required by the aggregator.
type Client interface {
	ListTools(ctx context.Context) ([]ToolDescriptor, error)
	CallTool(ctx context.Context, name string, arguments any) (*CallToolResult, error)
}

// StreamableHTTPClient implements MCP client calls over HTTP JSON-RPC.
type StreamableHTTPClient struct {
	BaseURL string
	Headers map[string]string
	Timeout time.Duration
	Logger  glog.Logger
}

const (
	mcpProtocolVersionHeader  = "mcp-protocol-version"
	mcpSessionIDHeader        = "mcp-session-id"
	mcpDefaultProtocolVersion = "2025-06-18"
	mcpAcceptHeaderValue      = "application/json, text/event-stream"
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
	if _, ok := merged[mcpSessionIDHeader]; !ok {
		merged[mcpSessionIDHeader] = "mcp-session-" + random.GetUUIDWithHyphens()
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

// ListTools calls the MCP tools/list method.
func (c *StreamableHTTPClient) ListTools(ctx context.Context) ([]ToolDescriptor, error) {
	var result struct {
		Tools []ToolDescriptor `json:"tools"`
	}
	if err := c.doRPC(ctx, "tools/list", nil, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool invokes a MCP tool by name.
func (c *StreamableHTTPClient) CallTool(ctx context.Context, name string, arguments any) (*CallToolResult, error) {
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	var result CallToolResult
	if err := c.doRPC(ctx, "tools/call", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// doRPC performs a JSON-RPC call against the MCP server.
func (c *StreamableHTTPClient) doRPC(ctx context.Context, method string, params any, out any) error {
	if c == nil {
		return errors.New("mcp client is nil")
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      random.GetUUID(),
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "marshal mcp request")
	}

	client := &http.Client{Timeout: c.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "create mcp request")
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	c.debugLogRequest(method, req.Header, data)

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "send mcp request")
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return errors.Wrap(readErr, "read mcp response body")
	}
	c.debugLogResponse(method, resp, body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf("mcp request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  map[string]any  `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return errors.Wrap(err, "decode mcp response")
	}
	if envelope.Error != nil {
		return errors.Errorf("mcp error: %v", envelope.Error)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return errors.Wrap(err, "unmarshal mcp result")
	}
	return nil
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
