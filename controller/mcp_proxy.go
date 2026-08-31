package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/mcp"
)

type mcpRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpInitializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type mcpCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Signature string         `json:"signature,omitempty"`
}

const (
	mcpServerName    = "one-api-mcp-proxy"
	mcpServerVersion = "1.1.0"
)

const (
	mcpErrParseError     = -32700
	mcpErrInvalidRequest = -32600
	mcpErrMethodNotFound = -32601
	mcpErrInvalidParams  = -32602
	mcpErrInternal       = -32603
)

// MCPProxy handles initialization-based MCP requests backed by the aggregate tool catalog.
//
// Parameters:
//   - c: the Gin context containing the authenticated Streamable HTTP request.
//
// Return values: none; the function writes the complete HTTP response.
func MCPProxy(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodPost:
		handleMCPPost(c)
	case http.MethodGet, http.MethodDelete:
		c.AbortWithStatus(http.StatusMethodNotAllowed)
	default:
		c.AbortWithStatus(http.StatusMethodNotAllowed)
	}
}

// handleMCPPost dispatches one legacy JSON-RPC request or notification.
//
// Parameters:
//   - c: the Gin context containing the authenticated request and request-scoped logger.
//
// Return values: none; the function writes a JSON-RPC response or HTTP 202 for notifications.
func handleMCPPost(c *gin.Context) {
	ctx := gmw.Ctx(c)
	var request mcpRPCRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&request); err != nil {
		respondMCPError(c, nil, mcpErrParseError, errors.Wrap(err, "decode mcp request"))
		return
	}
	isNotification := request.ID == nil

	switch strings.ToLower(strings.TrimSpace(request.Method)) {
	case "initialize":
		var params mcpInitializeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			respondMCPError(c, request.ID, mcpErrInvalidParams, errors.Wrap(err, "decode mcp initialize params"))
			return
		}
		respondMCPResult(c, request.ID, gin.H{
			"protocolVersion": mcp.NegotiateLegacyProtocolVersion(params.ProtocolVersion),
			"capabilities": gin.H{
				"tools": gin.H{"listChanged": false},
			},
			"serverInfo": gin.H{
				"name":    mcpServerName,
				"version": mcpServerVersion,
			},
		})
	case "notifications/initialized", "notifications/cancelled", "notifications/progress", "notifications/roots/list_changed":
		c.AbortWithStatus(http.StatusAccepted)
	case "ping":
		respondMCPResult(c, request.ID, gin.H{})
	case "tools/list":
		tools, err := listMCPToolsForUser(ctx, c)
		if err != nil {
			respondMCPError(c, request.ID, mcpErrInternal, err)
			return
		}
		respondMCPResult(c, request.ID, gin.H{"tools": tools})
	case "tools/call":
		var params mcpCallParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			respondMCPError(c, request.ID, mcpErrInvalidParams, errors.Wrap(err, "decode mcp call params"))
			return
		}
		result, err := callMCPToolForUser(ctx, c, params)
		if err != nil {
			respondMCPError(c, request.ID, mcpErrInternal, err)
			return
		}
		respondMCPResult(c, request.ID, result)
	default:
		if isNotification {
			c.AbortWithStatus(http.StatusAccepted)
			return
		}
		respondMCPError(c, request.ID, mcpErrMethodNotFound, errors.Errorf("unsupported method %s", request.Method))
	}
}

// listMCPToolsForUser returns lossless, policy-filtered, qualified descriptors for the authenticated user.
//
// Parameters:
//   - ctx: the request context controlling database and policy work.
//   - c: the Gin context containing the authenticated user.
//
// Return values:
//   - []mcp.ToolDescriptor: the deterministic aggregate tool catalog.
//   - error: a wrapped authentication, database, policy, or descriptor error.
func listMCPToolsForUser(ctx context.Context, c *gin.Context) ([]mcp.ToolDescriptor, error) {
	return listMCPToolDescriptorsForUser(ctx, c)
}

// callMCPToolForUser routes one legacy downstream request through the modern-first upstream client.
//
// Parameters:
//   - ctx: the request context controlling database and upstream work.
//   - c: the Gin context containing the authenticated user and request-scoped logger.
//   - params: the exact qualified tool name, arguments, and optional candidate signature.
//
// Return values:
//   - *mcp.CallToolResult: the normalized upstream result.
//   - error: a wrapped authentication, catalog, routing, execution, or billing error.
func callMCPToolForUser(ctx context.Context, c *gin.Context, params mcpCallParams) (*mcp.CallToolResult, error) {
	logger := gmw.GetLogger(c)
	user, err := getUserFromContext(c)
	if err != nil {
		return nil, errors.Wrap(err, "get user from context")
	}
	serverName, toolName := splitToolName(params.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(params.Name)
	}
	if toolName == "" {
		return nil, errors.New("tool name is required")
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}

	var servers []*model.MCPServer
	serverByID := make(map[int]*model.MCPServer)
	if serverName != "" {
		server, err := model.GetMCPServerByName(serverName)
		if err != nil {
			return nil, errors.Wrapf(err, "get mcp server by name %q", serverName)
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

	candidates, err := mcp.BuildToolCandidates(
		servers,
		toolsByServer,
		nil,
		user.MCPToolBlacklist,
		[]string{toolName},
		toolName,
		params.Signature,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "build mcp tool candidates for %q", toolName)
	}
	candidates = filterExactMCPToolCandidates(candidates, toolName)
	if len(candidates) == 0 {
		return nil, errors.Errorf("no eligible MCP tool found for exact name %q", toolName)
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
		callResult, err := client.CallToolLatestWithDescriptor(ctx, descriptor, params.Arguments)
		if err != nil {
			return nil, errors.Wrapf(err, "call mcp tool %q on server %d", candidate.Tool.Name, candidate.ServerID)
		}
		return callResult, nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "call mcp tool with fallback")
	}
	result = mcp.NormalizeCallToolResult(result)
	if !shouldBillMCPToolResult(result) {
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

// splitToolName separates an optional server qualifier from the exact upstream tool name.
//
// Parameters:
//   - value: a qualified name in server.tool form or an unqualified tool name.
//
// Return values:
//   - string: the optional server name before the first dot.
//   - string: the remaining exact tool name, which may itself contain dots.
func splitToolName(value string) (string, string) {
	value = strings.TrimSpace(value)
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return "", value
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

// toolPolicyMCPRequest adapts Gin request context values to the shared MCP policy interface.
type toolPolicyMCPRequest struct {
	ctx *gin.Context
}

// GetInt returns one integer policy value from the Gin context.
//
// Parameters:
//   - key: the context key to read.
//
// Return values:
//   - int: the decoded value or zero when missing.
func (r toolPolicyMCPRequest) GetInt(key string) int {
	return r.ctx.GetInt(key)
}

// GetString returns one string policy value from the Gin context.
//
// Parameters:
//   - key: the context key to read.
//
// Return values:
//   - string: the decoded value or an empty string when missing.
func (r toolPolicyMCPRequest) GetString(key string) string {
	return r.ctx.GetString(key)
}

// getUserFromContext resolves the authenticated user by UUID or legacy numeric id.
//
// Parameters:
//   - c: the Gin context populated by token authentication middleware.
//
// Return values:
//   - *model.User: the authenticated user model.
//   - error: a wrapped lookup error or missing-identity error.
func getUserFromContext(c *gin.Context) (*model.User, error) {
	userUUID := c.GetString(ctxkey.UserUUID)
	if userUUID != "" {
		user, err := model.GetUserByUUID(userUUID, false)
		if err != nil {
			return nil, errors.Wrap(err, "get user by uuid")
		}
		return user, nil
	}
	userID := c.GetInt(ctxkey.Id)
	if userID == 0 {
		return nil, errors.New("user id missing")
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return nil, errors.Wrap(err, "get user by id")
	}
	return user, nil
}

// buildMCPHeaders combines stored custom headers with request header overrides.
//
// Parameters:
//   - server: the configured upstream MCP server.
//   - c: the current downstream Gin request context.
//
// Return values:
//   - map[string]string: upstream custom headers after controlled override processing.
func buildMCPHeaders(server *model.MCPServer, c *gin.Context) map[string]string {
	headers := make(map[string]string)
	for key, value := range server.Headers {
		headers[key] = value
	}
	if value := c.GetHeader("X-MCP-Header-Override"); value != "" {
		var overrides map[string]string
		if err := json.Unmarshal([]byte(value), &overrides); err == nil {
			for key, override := range overrides {
				headers[key] = override
			}
		}
	}
	return headers
}

// resolveToolCost converts configured local pricing into the quota charge for one tool call.
//
// Parameters:
//   - server: the owning MCP server configuration.
//   - toolName: the exact upstream tool name.
//
// Return values:
//   - int: the non-negative quota charge.
func resolveToolCost(server *model.MCPServer, toolName string) int {
	pricing := server.ResolveToolPricing(toolName)
	quota := pricing.QuotaPerCall
	if quota <= 0 && pricing.UsdPerCall > 0 {
		quota = int(pricing.UsdPerCall * ratio.USD)
	}
	if quota < 0 {
		return 0
	}
	return quota
}

// recordMCPToolLog records one finalized successful tool call in the shared request log.
//
// Parameters:
//   - ctx: the request context carrying trace and cancellation metadata.
//   - c: the Gin context carrying token and channel metadata.
//   - userID: the authenticated user id.
//   - serverID: the selected MCP server id.
//   - toolName: the qualified tool name exposed by one-api.
//   - quota: the charged quota units.
//   - elapsed: total tool-call latency in milliseconds.
//
// Return values: none; persistence errors are handled by the shared model logging path.
func recordMCPToolLog(ctx context.Context, c *gin.Context, userID, serverID int, toolName string, quota int, elapsed int64) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestID := helper.GetRequestID(c)
	traceID := strings.TrimSpace(c.GetString(ctxkey.TraceID))
	if traceID == "" {
		traceID = tracing.TraceIDFromContext(ctx)
	}
	metadata := model.LogMetadataFromContext(c)
	metadata["mcp_server_id"] = serverID
	metadata["mcp_tool"] = toolName
	metadata["mcp_elapsed_ms"] = elapsed
	metadata["upstream_model"] = "mcp:" + toolName
	metadata["model_mapping"] = false
	metadata["model_mapping_source"] = ""
	metadata["model_mapping_original_model"] = "mcp:" + toolName
	metadata["model_mapping_mapped_model"] = "mcp:" + toolName
	model.RecordConsumeLog(ctx, &model.Log{
		UserId:           userID,
		CreatedAt:        model.GetTimestamp(),
		Type:             model.LogTypeConsume,
		Content:          fmt.Sprintf("MCP tool call %s", toolName),
		Username:         c.GetString(ctxkey.Username),
		TokenName:        c.GetString(ctxkey.TokenName),
		ModelName:        "mcp:" + toolName,
		Quota:            quota,
		PromptTokens:     0,
		CompletionTokens: 0,
		ChannelId:        c.GetInt(ctxkey.ChannelId),
		RequestId:        requestID,
		TraceId:          traceID,
		Metadata:         metadata,
	})
}

// respondMCPResult writes one successful initialization-based JSON-RPC response.
//
// Parameters:
//   - c: the Gin request context receiving the response.
//   - id: the decoded JSON-RPC request identifier.
//   - result: the result payload to encode.
//
// Return values: none; the function writes HTTP 200 JSON.
func respondMCPResult(c *gin.Context, id any, result any) {
	c.JSON(http.StatusOK, gin.H{"jsonrpc": "2.0", "id": id, "result": result})
}

// respondMCPError writes one legacy JSON-RPC error and redacts internal implementation details.
//
// Parameters:
//   - c: the Gin request context receiving the response and providing the request-scoped logger.
//   - id: the decoded JSON-RPC request identifier.
//   - code: the JSON-RPC error code.
//   - err: the underlying validation or internal error.
//
// Return values: none; the function writes HTTP 200 JSON.
func respondMCPError(c *gin.Context, id any, code int, err error) {
	message := "mcp request failed"
	if code == mcpErrInternal {
		if err != nil {
			gmw.GetLogger(c).Error("mcp internal request failure", zap.Error(err))
		}
		message = "internal MCP error"
	} else if err != nil {
		message = err.Error()
	}
	c.JSON(http.StatusOK, gin.H{
		"jsonrpc": "2.0",
		"id":      id,
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}
