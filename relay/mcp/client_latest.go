package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/random"
)

// DiscoverLatest calls server/discover using the MCP 2026-07-28 request model.
func (c *StreamableHTTPClient) DiscoverLatest(ctx context.Context) (*DiscoveryResult, error) {
	if c == nil {
		return nil, errors.New("mcp client is nil")
	}
	var result DiscoveryResult
	if err := c.doModernRPC(ctx, "server/discover", nil, "", nil, &result); err != nil {
		return nil, errors.Wrap(err, "mcp server/discover")
	}
	if result.ResultType == "" {
		result.ResultType = ResultTypeComplete
	}
	return &result, nil
}

// ListToolsLatest lists tools with MCP 2026-07-28 and falls back to the legacy handshake when required.
func (c *StreamableHTTPClient) ListToolsLatest(ctx context.Context) ([]ToolDescriptor, error) {
	if c == nil {
		return nil, errors.New("mcp client is nil")
	}
	if c.legacyInitialized() {
		return c.ListTools(ctx)
	}

	tools := make([]ToolDescriptor, 0)
	seenCursors := make(map[string]struct{})
	cursor := ""
	for page := 0; ; page++ {
		if page >= 1000 {
			return nil, errors.New("mcp tools/list exceeded 1000 pages")
		}
		var params map[string]any
		if cursor != "" {
			params = map[string]any{"cursor": cursor}
		}
		var result ListToolsResult
		err := c.doModernRPC(ctx, "tools/list", params, "", nil, &result)
		if err != nil {
			if page == 0 && IsModernFallbackCandidate(err) {
				return c.ListTools(ctx)
			}
			return nil, errors.Wrap(err, "mcp modern tools/list")
		}
		tools = append(tools, result.Tools...)
		nextCursor := strings.TrimSpace(result.NextCursor)
		if nextCursor == "" {
			break
		}
		if _, exists := seenCursors[nextCursor]; exists {
			return nil, errors.Errorf("mcp tools/list repeated cursor %q", nextCursor)
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}
	validTools, rejected := FilterValidToolDescriptors(tools)
	if c.Logger != nil {
		for _, rejection := range rejected {
			c.Logger.Warn("excluding invalid mcp tool descriptor",
				append(c.serverRef.Zap(), zap.String("tool", rejection.Name), zap.Error(rejection.Err))...)
		}
	}
	return validTools, nil
}

// CallToolLatest invokes a tool with MCP 2026-07-28 and falls back to the legacy handshake when required.
func (c *StreamableHTTPClient) CallToolLatest(ctx context.Context, name string, arguments any) (*CallToolResult, error) {
	return c.CallToolLatestWithDescriptor(ctx, ToolDescriptor{Name: name}, arguments)
}

// CallToolLatestWithDescriptor invokes a tool and derives schema-driven MCP parameter headers.
func (c *StreamableHTTPClient) CallToolLatestWithDescriptor(ctx context.Context, tool ToolDescriptor, arguments any) (*CallToolResult, error) {
	return c.CallToolLatestWithOptions(ctx, tool, arguments, CallToolRequestOptions{})
}

// CallToolLatestWithOptions invokes a tool and preserves MCP multi-round-trip retry fields.
func (c *StreamableHTTPClient) CallToolLatestWithOptions(ctx context.Context, tool ToolDescriptor, arguments any, options CallToolRequestOptions) (*CallToolResult, error) {
	if c == nil {
		return nil, errors.New("mcp client is nil")
	}
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		return nil, errors.New("mcp tool name is required")
	}
	if c.legacyInitialized() {
		if hasCallToolRequestOptions(options) {
			return nil, errors.New("legacy mcp servers cannot accept multi-round-trip tool retry fields")
		}
		return c.CallTool(ctx, name, arguments)
	}

	argumentMap, err := normalizeToolArguments(arguments)
	if err != nil {
		return nil, errors.Wrap(err, "normalize mcp tool arguments")
	}
	parameterHeaders, err := ToolArgumentHeaders(tool.InputSchema, argumentMap)
	if err != nil {
		return nil, errors.Wrapf(err, "derive mcp headers for tool %q", name)
	}
	params := map[string]any{
		"name":      name,
		"arguments": arguments,
	}
	if options.InputResponses != nil {
		params["inputResponses"] = options.InputResponses
	}
	if options.RequestState != "" {
		params["requestState"] = options.RequestState
	}
	var result CallToolResult
	err = c.doModernRPC(ctx, "tools/call", params, name, parameterHeaders, &result)
	if err != nil {
		if IsModernFallbackCandidate(err) && !hasCallToolRequestOptions(options) {
			return c.CallTool(ctx, name, arguments)
		}
		return nil, errors.Wrapf(err, "mcp modern tools/call %s", name)
	}
	return NormalizeCallToolResult(&result), nil
}

// hasCallToolRequestOptions reports whether a tool call carries modern multi-round-trip fields.
func hasCallToolRequestOptions(options CallToolRequestOptions) bool {
	return options.InputResponses != nil || options.RequestState != ""
}

// legacyInitialized reports whether the client has committed to the legacy initialization lifecycle.
func (c *StreamableHTTPClient) legacyInitialized() bool {
	if c == nil {
		return false
	}
	c.initMu.Lock()
	defer c.initMu.Unlock()
	return c.initialized
}

// doModernRPC performs one MCP 2026-07-28 JSON-RPC call with per-request metadata and mirrored headers.
func (c *StreamableHTTPClient) doModernRPC(ctx context.Context, method string, params map[string]any, name string, parameterHeaders http.Header, out any) error {
	if c == nil {
		return errors.New("mcp client is nil")
	}
	requestID := random.GetUUID()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  WithModernMeta(params),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "marshal modern mcp request")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(data))
	if err != nil {
		return errors.Wrap(err, "create modern mcp request")
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}
	req.Header.Set("Accept", mcpAcceptHeaderValue)
	req.Header.Del(SessionIDHeader)
	req.Header.Del("Last-Event-ID")
	req.Header.Set(ProtocolVersionHeader, ProtocolVersion)
	req.Header.Set(MethodHeader, method)
	if name != "" {
		req.Header.Set(NameHeader, EncodeMCPHeaderValue(name))
	} else {
		req.Header.Del(NameHeader)
	}
	for key, values := range parameterHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	c.debugLogRequest(method, req.Header, data)
	client := &http.Client{Timeout: c.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "send modern mcp request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read modern mcp response body")
	}
	c.debugLogResponse(method, resp, body)

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		body, err = extractModernSSEEnvelope(body, requestID)
		if err != nil {
			return errors.Wrap(err, "parse modern mcp sse response")
		}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return decodeModernProtocolError(resp.StatusCode, body)
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    any    `json:"data,omitempty"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return errors.Wrap(err, "decode modern mcp response")
	}
	if envelope.Error != nil {
		return &ProtocolError{
			HTTPStatus: resp.StatusCode,
			Code:       envelope.Error.Code,
			Message:    envelope.Error.Message,
			Data:       envelope.Error.Data,
			Body:       strings.TrimSpace(string(body)),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return errors.Wrap(err, "unmarshal modern mcp result")
	}
	return nil
}

// decodeModernProtocolError parses a JSON-RPC error when an HTTP-level modern request fails.
func decodeModernProtocolError(status int, body []byte) error {
	protocolErr := &ProtocolError{HTTPStatus: status, Body: strings.TrimSpace(string(body))}
	var envelope struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    any    `json:"data,omitempty"`
		} `json:"error,omitempty"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error != nil {
		protocolErr.Code = envelope.Error.Code
		protocolErr.Message = envelope.Error.Message
		protocolErr.Data = envelope.Error.Data
	}
	return protocolErr
}

// extractModernSSEEnvelope returns the JSON-RPC response event matching the request identifier.
func extractModernSSEEnvelope(body []byte, requestID string) ([]byte, error) {
	blocks := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n\n")
	var fallback []byte
	for _, block := range blocks {
		dataLines := make([]string, 0)
		for _, line := range strings.Split(block, "\n") {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			dataLines = append(dataLines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		if len(dataLines) == 0 {
			continue
		}
		candidate := []byte(strings.Join(dataLines, "\n"))
		fallback = candidate
		var envelope struct {
			ID any `json:"id"`
		}
		if json.Unmarshal(candidate, &envelope) != nil {
			continue
		}
		if id, ok := envelope.ID.(string); ok && id == requestID {
			return candidate, nil
		}
	}
	if len(fallback) != 0 {
		return fallback, nil
	}
	return nil, errors.New("sse response has no data fields")
}

// normalizeToolArguments converts JSON-compatible arguments to an object for header derivation.
func normalizeToolArguments(arguments any) (map[string]any, error) {
	if arguments == nil {
		return map[string]any{}, nil
	}
	if object, ok := arguments.(map[string]any); ok {
		return object, nil
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, errors.Wrap(err, "marshal mcp tool arguments")
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, errors.Wrap(err, "decode mcp tool arguments object")
	}
	if object == nil {
		return nil, errors.New("mcp tool arguments must be an object")
	}
	return object, nil
}
