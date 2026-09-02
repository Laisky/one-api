package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/random"
)

// DiscoverLatest calls server/discover using the MCP 2026-07-28 request model.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//
// Return values:
//   - *DiscoveryResult: the server's modern discovery result.
//   - error: a wrapped transport, protocol, or decoding error.
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

// ListToolsLatest lists every tool through modern pagination and falls back to the legacy lifecycle when required.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//
// Return values:
//   - []ToolDescriptor: validated tools collected across all result pages.
//   - error: a wrapped transport, protocol, pagination, or descriptor-validation error.
func (c *StreamableHTTPClient) ListToolsLatest(ctx context.Context) ([]ToolDescriptor, error) {
	if c == nil {
		return nil, errors.New("mcp client is nil")
	}
	if c.legacyInitialized() {
		tools, err := c.ListTools(ctx)
		if err != nil {
			return nil, err
		}
		return c.normalizeAndFilterToolDescriptors(tools), nil
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
				legacyTools, legacyErr := c.ListTools(ctx)
				if legacyErr != nil {
					return nil, legacyErr
				}
				return c.normalizeAndFilterToolDescriptors(legacyTools), nil
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
	return c.normalizeAndFilterToolDescriptors(tools), nil
}

// CallToolLatest invokes one tool with MCP 2026-07-28 and no multi-round-trip state.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - name: the exact case-sensitive upstream tool name.
//   - arguments: a JSON-compatible argument object; nil becomes an empty object.
//
// Return values:
//   - *CallToolResult: the normalized upstream result.
//   - error: a wrapped transport, protocol, validation, or decoding error.
func (c *StreamableHTTPClient) CallToolLatest(ctx context.Context, name string, arguments any) (*CallToolResult, error) {
	return c.CallToolLatestWithDescriptor(ctx, ToolDescriptor{Name: name}, arguments)
}

// CallToolLatestWithDescriptor invokes one tool and derives schema-driven request headers.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - tool: the upstream descriptor containing the exact name and input schema.
//   - arguments: a JSON-compatible argument object; nil becomes an empty object.
//
// Return values:
//   - *CallToolResult: the normalized upstream result.
//   - error: a wrapped transport, protocol, validation, or decoding error.
func (c *StreamableHTTPClient) CallToolLatestWithDescriptor(ctx context.Context, tool ToolDescriptor, arguments any) (*CallToolResult, error) {
	return c.CallToolLatestWithOptions(ctx, tool, arguments, CallToolRequestOptions{})
}

// CallToolLatestWithOptions invokes one tool and preserves multi-round-trip retry fields.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - tool: the upstream descriptor containing the exact name and input schema.
//   - arguments: a JSON-compatible argument object; nil becomes an empty object.
//   - options: optional input responses and opaque request state from an input_required result.
//
// Return values:
//   - *CallToolResult: the normalized upstream result.
//   - error: a wrapped transport, protocol, validation, or decoding error.
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
		"arguments": argumentMap,
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
			return c.CallTool(ctx, name, argumentMap)
		}
		return nil, errors.Wrapf(err, "mcp modern tools/call %s", name)
	}
	return NormalizeCallToolResult(&result), nil
}

// normalizeAndFilterToolDescriptors supplies legacy defaults and excludes invalid HTTP tool definitions.
//
// Parameters:
//   - tools: descriptors collected from one or more tools/list pages.
//
// Return values:
//   - []ToolDescriptor: valid descriptors with a non-nil object input schema.
func (c *StreamableHTTPClient) normalizeAndFilterToolDescriptors(tools []ToolDescriptor) []ToolDescriptor {
	for index := range tools {
		if tools[index].InputSchema == nil {
			tools[index].InputSchema = map[string]any{"type": "object"}
		}
	}
	validTools, rejected := FilterValidToolDescriptors(tools)
	if c != nil && c.Logger != nil {
		for _, rejection := range rejected {
			c.Logger.Warn("excluding invalid mcp tool descriptor",
				append(c.serverRef.Zap(), zap.String("tool", rejection.Name), zap.Error(rejection.Err))...)
		}
	}
	return validTools
}

// hasCallToolRequestOptions reports whether a call carries modern multi-round-trip fields.
//
// Parameters:
//   - options: the optional retry fields supplied by the caller.
//
// Return values:
//   - bool: true when legacy fallback cannot represent the request.
func hasCallToolRequestOptions(options CallToolRequestOptions) bool {
	return options.InputResponses != nil || options.RequestState != ""
}

// legacyInitialized reports whether the client has committed to the legacy session lifecycle.
//
// Parameters: none.
//
// Return values:
//   - bool: true after a successful initialize exchange.
func (c *StreamableHTTPClient) legacyInitialized() bool {
	if c == nil {
		return false
	}
	c.initMu.Lock()
	defer c.initMu.Unlock()
	return c.initialized
}

// doModernRPC performs one stateless MCP 2026-07-28 JSON-RPC request.
//
// Parameters:
//   - ctx: the request context controlling cancellation and deadlines.
//   - method: the exact JSON-RPC method mirrored into the HTTP request.
//   - params: method-specific parameters before required modern metadata is attached.
//   - name: an optional tool or resource name mirrored into the HTTP request.
//   - parameterHeaders: schema-derived MCP parameter headers for this request only.
//   - out: the destination for a successful result, or nil to discard it.
//
// Return values:
//   - error: a wrapped transport, size, correlation, protocol, or decoding error.
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
	for key, value := range c.headerSnapshot() {
		req.Header.Set(key, value)
	}
	for key := range req.Header {
		if strings.HasPrefix(strings.ToLower(key), strings.ToLower(ParameterHeaderPrefix)) {
			req.Header.Del(key)
		}
	}
	requestHeaderSet := func(key, value string) {
		if value == "" {
			req.Header.Del(key)
			return
		}
		req.Header.Set(key, value)
	}
	requestHeaderSet("Accept", mcpAcceptHeaderValue)
	req.Header.Del(SessionIDHeader)
	req.Header.Del("Last-Event-ID")
	requestHeaderSet(ProtocolVersionHeader, ProtocolVersion)
	requestHeaderSet(MethodHeader, method)
	if name != "" {
		requestHeaderSet(NameHeader, EncodeMCPHeaderValue(name))
	} else {
		req.Header.Del(NameHeader)
	}
	for key, values := range parameterHeaders {
		if len(values) != 1 {
			return errors.Errorf("mcp parameter header %s must contain exactly one value", key)
		}
		req.Header.Set(key, values[0])
	}

	c.debugLogRequest(method, req.Header, data)
	client := c.httpClient()
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "send modern mcp request")
	}
	defer resp.Body.Close()

	body, err := readMCPResponseBody(resp.Body)
	if err != nil {
		return errors.Wrap(err, "read modern mcp response body")
	}
	c.debugLogResponse(method, resp, body)

	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		body, err = extractMCPResponseEnvelope(body, requestID)
		if err != nil {
			return errors.Wrap(err, "parse modern mcp SSE response")
		}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return decodeModernProtocolError(resp.StatusCode, body)
	}

	envelope, err := parseMCPResponseEnvelope(body, requestID)
	if err != nil {
		return err
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

// decodeModernProtocolError parses an HTTP-level modern MCP error response.
//
// Parameters:
//   - status: the non-success HTTP status returned by the peer.
//   - body: the bounded response body.
//
// Return values:
//   - error: a ProtocolError retaining the HTTP and JSON-RPC details available to fallback policy.
func decodeModernProtocolError(status int, body []byte) error {
	protocolErr := &ProtocolError{HTTPStatus: status, Body: strings.TrimSpace(string(body))}
	var envelope mcpJSONRPCEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error != nil {
		protocolErr.Code = envelope.Error.Code
		protocolErr.Message = envelope.Error.Message
		protocolErr.Data = envelope.Error.Data
	}
	return protocolErr
}

// normalizeToolArguments converts JSON-compatible arguments into a non-nil object.
//
// Parameters:
//   - arguments: nil, a map, or another value that JSON can decode as an object.
//
// Return values:
//   - map[string]any: the normalized argument object.
//   - error: a wrapped encoding or type error when arguments are not a JSON object.
func normalizeToolArguments(arguments any) (map[string]any, error) {
	if arguments == nil {
		return map[string]any{}, nil
	}
	if object, ok := arguments.(map[string]any); ok {
		if object == nil {
			return map[string]any{}, nil
		}
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
