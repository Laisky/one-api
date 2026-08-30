package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

const modernMCPMaxRequestBytes = 4 << 20

// MCPProxyLatest dispatches MCP 2026-07-28 requests while preserving the legacy endpoint behavior.
func MCPProxyLatest(c *gin.Context) {
	if err := validateModernMCPOrigin(c.Request); err != nil {
		respondMCPModernError(c, nil, http.StatusForbidden, mcpErrInvalidRequest, err, nil)
		return
	}
	if c.Request.Method != http.MethodPost {
		MCPProxy(c)
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, modernMCPMaxRequestBytes+1))
	if err != nil {
		respondMCPModernError(c, nil, http.StatusBadRequest, mcpErrParseError, errors.Wrap(err, "read mcp request"), nil)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(body) > modernMCPMaxRequestBytes {
		respondMCPModernError(c, nil, http.StatusRequestEntityTooLarge, mcpErrInvalidRequest, errors.New("mcp request body exceeds 4 MiB"), nil)
		return
	}

	var req mcpRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		if c.GetHeader(mcp.ProtocolVersionHeader) == "" {
			MCPProxy(c)
			return
		}
		respondMCPModernError(c, nil, http.StatusBadRequest, mcpErrParseError, errors.Wrap(err, "decode modern mcp request"), nil)
		return
	}
	if !isModernMCPRequest(c, req) {
		MCPProxy(c)
		return
	}
	if err := validateModernMCPRequest(c, req); err != nil {
		respondModernValidationError(c, req.ID, err)
		return
	}
	handleModernMCPPost(c, req)
}

// modernMCPRequestMeta extracts the per-request metadata used to negotiate modern MCP requests.
type modernMCPRequestMeta struct {
	ProtocolVersion    string                  `json:"io.modelcontextprotocol/protocolVersion"`
	ClientInfo         *mcp.ImplementationInfo `json:"io.modelcontextprotocol/clientInfo,omitempty"`
	ClientCapabilities map[string]any          `json:"io.modelcontextprotocol/clientCapabilities"`
}

// modernMCPParamsEnvelope extracts the reserved _meta object without constraining method-specific parameters.
type modernMCPParamsEnvelope struct {
	Meta modernMCPRequestMeta `json:"_meta"`
}

// modernMCPCallParams contains tool invocation and multi-round-trip retry fields.
type modernMCPCallParams struct {
	Name           string         `json:"name"`
	Arguments      map[string]any `json:"arguments"`
	Signature      string         `json:"signature,omitempty"`
	InputResponses map[string]any `json:"inputResponses,omitempty"`
	RequestState   string         `json:"requestState,omitempty"`
}

// modernMCPValidationError carries the HTTP and JSON-RPC details for a rejected modern request.
type modernMCPValidationError struct {
	Status int
	Code   int
	Err    error
	Data   any
}

// Error returns the underlying validation message.
func (e *modernMCPValidationError) Error() string {
	if e == nil || e.Err == nil {
		return "invalid modern mcp request"
	}
	return e.Err.Error()
}

// isModernMCPRequest reports whether a request uses the MCP 2026-07-28 request model.
func isModernMCPRequest(c *gin.Context, req mcpRPCRequest) bool {
	if strings.TrimSpace(req.Method) == "server/discover" {
		return true
	}
	if strings.TrimSpace(c.GetHeader(mcp.ProtocolVersionHeader)) != "" {
		return true
	}
	var params modernMCPParamsEnvelope
	return json.Unmarshal(req.Params, &params) == nil && strings.TrimSpace(params.Meta.ProtocolVersion) != ""
}

// validateModernMCPRequest validates protocol metadata, mirrored headers, and request identity.
func validateModernMCPRequest(c *gin.Context, req mcpRPCRequest) error {
	if req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.New("jsonrpc must be 2.0 and method is required")}
	}
	if req.ID == nil && isModernMCPRequestMethod(req.Method) {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.New("modern mcp requests require a non-null id")}
	}
	if req.ID != nil && !isValidModernMCPRequestID(req.ID) {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.New("modern mcp request id must be a string or integer")}
	}
	var params modernMCPParamsEnvelope
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.Wrap(err, "decode modern mcp metadata")}
	}
	bodyVersion := strings.TrimSpace(params.Meta.ProtocolVersion)
	headerVersion, err := singleMCPHeaderValue(c.Request.Header, mcp.ProtocolVersionHeader)
	if err != nil || bodyVersion == "" {
		if err == nil {
			err = errors.New("modern mcp requests require protocol version metadata")
		}
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
	}
	if bodyVersion != headerVersion {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: errors.New("MCP-Protocol-Version does not match request _meta")}
	}
	if bodyVersion != mcp.ProtocolVersion {
		return &modernMCPValidationError{
			Status: http.StatusBadRequest,
			Code:   mcp.ErrorCodeUnsupportedProtocolVersion,
			Err:    errors.Errorf("unsupported mcp protocol version %q", bodyVersion),
			Data: gin.H{
				"supported": []string{mcp.ProtocolVersion},
				"requested": bodyVersion,
			},
		}
	}
	if params.Meta.ClientCapabilities == nil {
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidRequest, Err: errors.New("modern mcp requests require client capabilities in _meta")}
	}
	headerMethod, err := singleMCPHeaderValue(c.Request.Header, mcp.MethodHeader)
	if err != nil || headerMethod != req.Method {
		if err == nil {
			err = errors.New("MCP-Method does not match the JSON-RPC method")
		}
		return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
	}
	if req.Method == "tools/call" {
		var params modernMCPCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcpErrInvalidParams, Err: errors.Wrap(err, "decode mcp call params")}
		}
		headerName, err := singleMCPHeaderValue(c.Request.Header, mcp.NameHeader)
		if err != nil {
			return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
		}
		decodedName, err := mcp.DecodeMCPHeaderValue(headerName)
		if err != nil || decodedName != params.Name {
			if err == nil {
				err = errors.New("MCP-Name does not match tools/call params.name")
			}
			return &modernMCPValidationError{Status: http.StatusBadRequest, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
		}
	}
	return nil
}

// isModernMCPRequestMethod reports whether a method is a core request supported by this endpoint.
func isModernMCPRequestMethod(method string) bool {
	switch method {
	case "server/discover", "tools/list", "tools/call":
		return true
	default:
		return false
	}
}

// isValidModernMCPRequestID reports whether a JSON value is a string or integer request identifier.
func isValidModernMCPRequestID(id any) bool {
	switch typed := id.(type) {
	case string:
		return true
	case float64:
		return !math.IsNaN(typed) && !math.IsInf(typed, 0) && math.Trunc(typed) == typed
	default:
		return false
	}
}

// singleMCPHeaderValue returns one required header value and rejects missing or repeated values.
func singleMCPHeaderValue(headers http.Header, name string) (string, error) {
	values := headers.Values(name)
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return "", errors.Errorf("%s must occur exactly once", name)
	}
	return values[0], nil
}

// validateModernMCPOrigin protects the HTTP endpoint from DNS rebinding through browser Origin requests.
func validateModernMCPOrigin(request *http.Request) error {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return nil
	}
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("invalid Origin header")
	}
	if !strings.EqualFold(parsed.Host, strings.TrimSpace(request.Host)) {
		return errors.Errorf("Origin host %q does not match the MCP endpoint host", parsed.Host)
	}
	return nil
}

// respondModernValidationError writes a modern protocol validation failure.
func respondModernValidationError(c *gin.Context, id any, err error) {
	validationErr, ok := err.(*modernMCPValidationError)
	if !ok || validationErr == nil {
		respondMCPModernError(c, id, http.StatusBadRequest, mcpErrInvalidRequest, err, nil)
		return
	}
	respondMCPModernError(c, id, validationErr.Status, validationErr.Code, validationErr.Err, validationErr.Data)
}

// handleModernMCPPost serves modern discovery, tool listing, and tool calls without an initialize handshake.
func handleModernMCPPost(c *gin.Context, req mcpRPCRequest) {
	ctx := gmw.Ctx(c)
	switch strings.TrimSpace(req.Method) {
	case "server/discover":
		respondMCPModernResult(c, req.ID, mcp.DiscoveryResult{
			ResultType:        mcp.ResultTypeComplete,
			SupportedVersions: []string{mcp.ProtocolVersion},
			Capabilities: gin.H{
				"tools": gin.H{"listChanged": false},
			},
			TTLMS:      int64(time.Hour / time.Millisecond),
			CacheScope: mcp.CacheScopePrivate,
			Meta:       mcp.ServerResponseMeta(mcpServerName, mcpServerVersion),
		})
	case "tools/list":
		tools, err := listMCPToolsForUser(ctx, c)
		if err != nil {
			respondMCPModernError(c, req.ID, http.StatusOK, mcpErrInternal, err, nil)
			return
		}
		tools = normalizeModernToolDescriptors(tools)
		tools, rejected := mcp.FilterValidToolDescriptors(tools)
		if len(rejected) != 0 {
			logger := gmw.GetLogger(c)
			for _, rejection := range rejected {
				logger.Warn("excluding invalid mcp tool descriptor", zap.String("tool", rejection.Name), zap.Error(rejection.Err))
			}
		}
		sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
		respondMCPModernResult(c, req.ID, gin.H{
			"resultType": mcp.ResultTypeComplete,
			"tools":      tools,
			"ttlMs":      int64(time.Minute / time.Millisecond),
			"cacheScope": mcp.CacheScopePrivate,
		})
	case "tools/call":
		var params modernMCPCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			respondMCPModernError(c, req.ID, http.StatusBadRequest, mcpErrInvalidParams, errors.Wrap(err, "decode mcp call params"), nil)
			return
		}
		descriptor, err := findModernMCPToolDescriptor(ctx, c, params.Name)
		if err != nil {
			respondMCPModernError(c, req.ID, http.StatusOK, mcpErrInvalidParams, err, nil)
			return
		}
		if err := mcp.ValidateToolArgumentHeaders(c.Request.Header, descriptor.InputSchema, params.Arguments); err != nil {
			respondMCPModernError(c, req.ID, http.StatusBadRequest, mcp.ErrorCodeHeaderMismatch, err, nil)
			return
		}
		result, err := callMCPToolForUserLatest(ctx, c, params)
		if err != nil {
			respondMCPModernError(c, req.ID, http.StatusOK, mcpErrInternal, err, nil)
			return
		}
		respondMCPModernResult(c, req.ID, mcp.NormalizeCallToolResult(result))
	default:
		if req.ID == nil {
			c.AbortWithStatus(http.StatusAccepted)
			return
		}
		respondMCPModernError(c, req.ID, http.StatusNotFound, mcpErrMethodNotFound, errors.Errorf("unsupported method %s", req.Method), nil)
	}
}

// normalizeModernToolDescriptors supplies the required empty-object schema for legacy stored tools.
func normalizeModernToolDescriptors(tools []mcp.ToolDescriptor) []mcp.ToolDescriptor {
	for index := range tools {
		if tools[index].InputSchema == nil {
			tools[index].InputSchema = map[string]any{"type": "object"}
		}
	}
	return tools
}

// findModernMCPToolDescriptor resolves a qualified tool name for modern header validation.
func findModernMCPToolDescriptor(ctx context.Context, c *gin.Context, name string) (mcp.ToolDescriptor, error) {
	tools, err := listMCPToolsForUser(ctx, c)
	if err != nil {
		return mcp.ToolDescriptor{}, errors.Wrap(err, "list mcp tools for header validation")
	}
	tools = normalizeModernToolDescriptors(tools)
	tools, _ = mcp.FilterValidToolDescriptors(tools)
	for _, tool := range tools {
		if tool.Name == name {
			return tool, nil
		}
	}
	return mcp.ToolDescriptor{}, errors.Errorf("no eligible MCP tool found for %q", name)
}

// callMCPToolForUserLatest invokes an MCP tool through modern-first upstream negotiation and applies billing.
func callMCPToolForUserLatest(ctx context.Context, c *gin.Context, params modernMCPCallParams) (*mcp.CallToolResult, error) {
	logger := gmw.GetLogger(c)
	user, err := getUserFromContext(c)
	if err != nil {
		return nil, errors.Wrap(err, "get user from context")
	}

	serverLabel, toolName := splitToolName(params.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(params.Name)
	}
	if toolName == "" {
		return nil, errors.New("tool name is required")
	}

	var servers []*model.MCPServer
	serverByID := make(map[int]*model.MCPServer)
	if serverLabel != "" {
		server, err := model.GetMCPServerByName(serverLabel)
		if err != nil {
			return nil, errors.Wrapf(err, "get mcp server by name %q", serverLabel)
		}
		servers = []*model.MCPServer{server}
		serverByID[server.Id] = server
	} else {
		servers, err = model.ListEnabledMCPServers()
		if err != nil {
			return nil, errors.Wrap(err, "list enabled mcp servers")
		}
		for _, server := range servers {
			if server != nil {
				serverByID[server.Id] = server
			}
		}
	}

	toolsByServer := make(map[int][]*model.MCPTool, len(servers))
	for _, server := range servers {
		if server == nil {
			continue
		}
		tools, err := model.GetMCPToolsByServerID(server.Id)
		if err != nil {
			return nil, errors.Wrapf(err, "get mcp tools for server %d", server.Id)
		}
		toolsByServer[server.Id] = tools
	}

	candidates, err := mcp.BuildToolCandidates(servers, toolsByServer, nil, user.MCPToolBlacklist, []string{toolName}, toolName, params.Signature)
	if err != nil {
		return nil, errors.Wrapf(err, "build mcp tool candidates for %q", toolName)
	}
	if len(candidates) == 0 {
		return nil, errors.New("no eligible MCP tool found")
	}

	startedAt := time.Now()
	selected, result, err := mcp.CallWithFallback(ctx, candidates, func(ctx context.Context, candidate mcp.ToolCandidate) (*mcp.CallToolResult, error) {
		server := serverByID[candidate.ServerID]
		if server == nil {
			return nil, errors.New("mcp server not loaded")
		}
		descriptor, err := descriptorForMCPTool(candidate.Tool)
		if err != nil {
			return nil, errors.Wrapf(err, "build descriptor for tool %q", candidate.Tool.Name)
		}
		client := mcp.NewStreamableHTTPClientWithLogger(server, nil, time.Duration(config.MCPToolCallTimeoutSec)*time.Second, logger)
		callResult, err := client.CallToolLatestWithOptions(ctx, descriptor, params.Arguments, mcp.CallToolRequestOptions{
			InputResponses: params.InputResponses,
			RequestState:   params.RequestState,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "call mcp tool %q on server %d", candidate.Tool.Name, candidate.ServerID)
		}
		return callResult, nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "call mcp tool with fallback")
	}
	result = mcp.NormalizeCallToolResult(result)
	if result.IsError {
		return result, nil
	}

	server := serverByID[selected.ServerID]
	if server == nil {
		return nil, errors.New("mcp server not loaded")
	}
	cost := resolveToolCost(server, selected.Tool.Name)
	if cost > 0 {
		if err := model.DecreaseUserQuota(ctx, user.Id, cost); err != nil {
			return nil, errors.Wrap(err, "decrease user quota for mcp tool call")
		}
		model.UpdateUserUsedQuotaAndRequestCountWithContext(ctx, user.Id, cost)
	}
	qualifiedName := server.Name + "." + selected.Tool.Name
	recordMCPToolLog(ctx, c, user.Id, server.Id, qualifiedName, cost, time.Since(startedAt).Milliseconds())
	return result, nil
}

// descriptorForMCPTool converts synchronized database metadata into an MCP wire descriptor.
func descriptorForMCPTool(tool *model.MCPTool) (mcp.ToolDescriptor, error) {
	if tool == nil {
		return mcp.ToolDescriptor{}, errors.New("mcp tool is nil")
	}
	descriptor := mcp.ToolDescriptor{
		Name:        tool.Name,
		Title:       tool.DisplayName,
		Description: tool.Description,
		InputSchema: map[string]any{"type": "object"},
	}
	if tool.InputSchema != "" {
		if err := json.Unmarshal([]byte(tool.InputSchema), &descriptor.InputSchema); err != nil {
			return mcp.ToolDescriptor{}, errors.Wrap(err, "decode mcp tool input schema")
		}
	}
	return descriptor, nil
}

// respondMCPModernResult writes a JSON-RPC result with protocol-required result and server metadata.
func respondMCPModernResult(c *gin.Context, id any, result any) {
	encoded, err := json.Marshal(result)
	if err != nil {
		respondMCPModernError(c, id, http.StatusOK, mcpErrInternal, errors.Wrap(err, "marshal modern mcp result"), nil)
		return
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		respondMCPModernError(c, id, http.StatusOK, mcpErrInternal, errors.Wrap(err, "normalize modern mcp result"), nil)
		return
	}
	if normalized["resultType"] == nil || normalized["resultType"] == "" {
		normalized["resultType"] = mcp.ResultTypeComplete
	}
	if content, exists := normalized["content"]; exists && content == nil {
		delete(normalized, "content")
	}
	promoteModernResultAlias(normalized, "structured_content", "structuredContent")
	promoteModernResultAlias(normalized, "is_error", "isError")
	promoteModernResultAlias(normalized, "input_requests", "inputRequests")
	promoteModernResultAlias(normalized, "request_state", "requestState")
	meta, _ := normalized["_meta"].(map[string]any)
	if meta == nil {
		meta = make(map[string]any)
	}
	meta[mcp.MetaServerInfoKey] = gin.H{"name": mcpServerName, "version": mcpServerVersion}
	normalized["_meta"] = meta
	c.JSON(http.StatusOK, gin.H{"jsonrpc": "2.0", "id": id, "result": normalized})
}

// promoteModernResultAlias moves a legacy result field to its modern camelCase name.
func promoteModernResultAlias(result map[string]any, legacyName, modernName string) {
	if value, exists := result[legacyName]; exists {
		if _, modernExists := result[modernName]; !modernExists {
			result[modernName] = value
		}
		delete(result, legacyName)
	}
}

// respondMCPModernError writes a JSON-RPC error with the HTTP status required by the modern transport.
func respondMCPModernError(c *gin.Context, id any, status int, code int, err error, data any) {
	message := "mcp request failed"
	if err != nil {
		message = err.Error()
	}
	errorObject := gin.H{"code": code, "message": message}
	if data != nil {
		errorObject["data"] = data
	}
	c.JSON(status, gin.H{"jsonrpc": "2.0", "id": id, "error": errorObject})
}
