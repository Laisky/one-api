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

// handleMCPPost dispatches one initialization-based JSON-RPC request or notification.
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
	if request.JSONRPC != "2.0" || strings.TrimSpace(request.Method) == "" {
		respondMCPError(c, request.ID, mcpErrInvalidRequest, errors.New("jsonrpc must be 2.0 and method is required"))
		return
	}
	isNotification := request.ID == nil

	switch strings.ToLower(strings.TrimSpace(request.Method)) {
	case "initialize":
		if isNotification {
			respondMCPError(c, nil, mcpErrInvalidRequest, errors.New("initialize requires a request id"))
			return
		}
		var params mcpInitializeParams
		if len(request.Params) != 0 {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				respondMCPError(c, request.ID, mcpErrInvalidParams, errors.Wrap(err, "decode mcp initialize params"))
				return
			}
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
		if isNotification {
			c.AbortWithStatus(http.StatusAccepted)
			return
		}
		respondMCPResult(c, request.ID, gin.H{})
	case "tools/list":
		if isNotification {
			c.AbortWithStatus(http.StatusAccepted)
			return
		}
		tools, err := listMCPToolsForUser(ctx, c)
		if err != nil {
			respondMCPError(c, request.ID, mcpErrInternal, err)
			return
		}
		respondMCPResult(c, request.ID, gin.H{"tools": tools})
	case "tools/call":
		if isNotification {
			c.AbortWithStatus(http.StatusAccepted)
			return
		}
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

	serverLabel, toolName := splitToolName(params.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(params.Name)
	}
	if toolName == "" {
		return nil, errors.WithStack(errors.New("tool name is required"))
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}

	servers, serverByID, err := loadMCPCallServers(serverLabel)
	if err != nil {
		return nil, err
	}
	toolsByServer, err := loadMCPToolsByServer(servers)
	if err != nil {
		return nil, err
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

	startedAt := time.Now() // Preserve the monotonic component for elapsed-time measurement.
	selected, result, err := mcp.CallWithFallback(ctx, candidates, func(ctx context.Context, candidate mcp.ToolCandidate) (*mcp.CallToolResult, error) {
		server := serverByID[candidate.ServerID]
		if server == nil {
			return nil, errors.WithStack(errors.New("mcp server not loaded"))
		}
		descriptor, err := descriptorForMCPTool(candidate.Tool)
		if err != nil {
			return nil, errors.Wrapf(err, "build descriptor for tool %q", candidate.Tool.Name)
		}
		client := mcp.NewStreamableHTTPClientWithLogger(
			server,
			nil,
			time.Duration(config.MCPToolCallTimeoutSec)*time.Second,
			logger,
		)
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
	if err := chargeAndRecordMCPToolCall(ctx, c, user.Id, serverByID, selected, startedAt); err != nil {
		return nil, err
	}
	return result, nil
}

// loadMCPCallServers loads either one explicitly selected server or every enabled server.
//
// Parameters:
//   - serverLabel: an optional configured MCP server name.
//
// Return values:
//   - []*model.MCPServer: candidate servers in repository-defined order.
//   - map[int]*model.MCPServer: candidate servers indexed by internal id.
//   - error: a wrapped server lookup error.
func loadMCPCallServers(serverLabel string) ([]*model.MCPServer, map[int]*model.MCPServer, error) {
	serverByID := make(map[int]*model.MCPServer)
	if serverLabel != "" {
		server, err := model.GetMCPServerByName(serverLabel)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "get mcp server by name %q", serverLabel)
		}
		serverByID[server.Id] = server
		return []*model.MCPServer{server}, serverByID, nil
	}

	servers, err := model.ListEnabledMCPServers()
	if err != nil {
		return nil, nil, errors.Wrap(err, "list enabled mcp servers")
	}
	for _, server := range servers {
		if server != nil {
			serverByID[server.Id] = server
		}
	}
	return servers, serverByID, nil
}

// loadMCPToolsByServer loads synchronized tool rows for each candidate MCP server.
//
// Parameters:
//   - servers: candidate MCP servers.
//
// Return values:
//   - map[int][]*model.MCPTool: synchronized tools grouped by owning server id.
//   - error: a wrapped database error.
func loadMCPToolsByServer(servers []*model.MCPServer) (map[int][]*model.MCPTool, error) {
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
	return toolsByServer, nil
}

// chargeAndRecordMCPToolCall applies quota and writes one finalized tool-call audit log.
//
// Parameters:
//   - ctx: the request context controlling quota persistence.
//   - c: the Gin context carrying request identity and tracing metadata.
//   - userID: the authenticated user's internal id.
//   - serverByID: loaded server configurations indexed by internal id.
//   - selected: the successful tool candidate.
//   - startedAt: the beginning of the logical tool call.
//
// Return values:
//   - error: a wrapped server lookup or quota update error.
func chargeAndRecordMCPToolCall(ctx context.Context, c *gin.Context, userID int, serverByID map[int]*model.MCPServer, selected mcp.ToolCandidate, startedAt time.Time) error {
	server := serverByID[selected.ServerID]
	if server == nil {
		return errors.WithStack(errors.New("mcp server not loaded"))
	}
	cost := resolveToolCost(server, selected.Tool.Name)
	if cost > 0 {
		if err := model.DecreaseUserQuota(ctx, userID, cost); err != nil {
			return errors.Wrap(err, "decrease user quota for mcp tool call")
		}
		model.UpdateUserUsedQuotaAndRequestCountWithContext(ctx, userID, cost)
	}
	qualifiedName := server.Name + "." + selected.Tool.Name
	recordMCPToolLog(ctx, c, userID, server.Id, qualifiedName, cost, helper.CalcElapsedTime(startedAt))
	return nil
}

// resolveToolCost determines the quota charge for one exact MCP tool name.
//
// Parameters:
//   - server: the owning MCP server configuration.
//   - toolName: the exact upstream tool name.
//
// Return values:
//   - int64: the configured non-negative quota cost.
func resolveToolCost(server *model.MCPServer, toolName string) int64 {
	if server == nil {
		return 0
	}
	pricing, exists := server.ToolPricing[toolName]
	if !exists {
		pricing = server.ToolPricing[strings.ToLower(toolName)]
	}
	if pricing.QuotaPerCall > 0 {
		return pricing.QuotaPerCall
	}
	if pricing.UsdPerCall > 0 {
		return int64(pricing.UsdPerCall * float64(ratio.QuotaPerUsd))
	}
	return 0
}

// mcpServerLabel renders an MCP server using its public name and UUID.
//
// Parameters:
//   - ctx: the request context reserved for future context-aware store access.
//   - serverID: the internal MCP server id.
//
// Return values:
//   - string: "<name> <uuid>" when resolvable, otherwise "unknown".
func mcpServerLabel(ctx context.Context, serverID int) string {
	_ = ctx
	server, err := model.GetMCPServerByID(serverID)
	if err != nil || server == nil {
		return "unknown"
	}
	parts := make([]string, 0, 2)
	if name := strings.TrimSpace(server.Name); name != "" {
		parts = append(parts, name)
	}
	if uuid := strings.TrimSpace(server.UUID); uuid != "" {
		parts = append(parts, uuid)
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, " ")
}

// recordMCPToolLog records one finalized MCP tool invocation in the tool audit stream.
//
// Parameters:
//   - ctx: the request context carrying cancellation and trace state.
//   - c: the Gin context carrying authenticated UUIDs and request identifiers.
//   - userID: the authenticated user id.
//   - serverID: the selected MCP server id.
//   - toolName: the qualified tool name exposed by one-api.
//   - cost: the charged quota units.
//   - elapsedMs: total logical tool-call latency in milliseconds.
//
// Return values: none; the shared model logging path owns persistence handling.
func recordMCPToolLog(ctx context.Context, c *gin.Context, userID int, serverID int, toolName string, cost int64, elapsedMs int64) {
	model.RecordToolLog(ctx, &model.Log{
		UserId:      userID,
		UserUUID:    model.StringPtrIfNotEmpty(c.GetString(ctxkey.UserUUID)),
		TokenUUID:   model.StringPtrIfNotEmpty(c.GetString(ctxkey.TokenUUID)),
		ModelName:   toolName,
		Quota:       int(cost),
		Content:     fmt.Sprintf("MCP tool call: %s (server %s)", toolName, mcpServerLabel(ctx, serverID)),
		RequestId:   c.GetString(ctxkey.RequestId),
		TraceId:     tracing.GetTraceID(c),
		IsStream:    false,
		ElapsedTime: elapsedMs,
	})
}

// getUserFromContext loads the authenticated user from request context.
//
// Parameters:
//   - c: the Gin context populated by token authentication middleware.
//
// Return values:
//   - *model.User: the authenticated user.
//   - error: a wrapped lookup error or missing-identity error.
func getUserFromContext(c *gin.Context) (*model.User, error) {
	if userObject, exists := c.Get(ctxkey.UserObj); exists {
		if user, ok := userObject.(*model.User); ok && user != nil {
			return user, nil
		}
	}
	userID := c.GetInt(ctxkey.Id)
	if userID == 0 {
		return nil, errors.WithStack(errors.New("user id missing"))
	}
	user, err := model.GetUserById(userID, true)
	if err != nil {
		return nil, errors.Wrapf(err, "get user by id %d", userID)
	}
	return user, nil
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
		logger := gmw.GetLogger(c)
		if err != nil {
			logger.Error("mcp internal request failure", zap.Error(err))
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
