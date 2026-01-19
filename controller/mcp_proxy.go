package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/mcp"
)

type mcpRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Signature string         `json:"signature,omitempty"`
}

// MCPProxy handles MCP Streamable HTTP requests backed by configured MCP servers.
func MCPProxy(c *gin.Context) {
	ctx := gmw.Ctx(c)
	var req mcpRPCRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		respondMCPError(c, req.ID, errors.Wrap(err, "decode mcp request"))
		return
	}

	switch strings.ToLower(strings.TrimSpace(req.Method)) {
	case "tools/list":
		tools, err := listMCPToolsForUser(ctx, c)
		if err != nil {
			respondMCPError(c, req.ID, err)
			return
		}
		respondMCPResult(c, req.ID, gin.H{"tools": tools})
	case "tools/call":
		var params mcpCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			respondMCPError(c, req.ID, errors.Wrap(err, "decode mcp call params"))
			return
		}
		result, err := callMCPToolForUser(ctx, c, params)
		if err != nil {
			respondMCPError(c, req.ID, err)
			return
		}
		respondMCPResult(c, req.ID, result)
	default:
		respondMCPError(c, req.ID, errors.Errorf("unsupported method %s", req.Method))
	}
}

// listMCPToolsForUser returns the allowed MCP tools for the authenticated user.
func listMCPToolsForUser(ctx context.Context, c *gin.Context) ([]mcp.ToolDescriptor, error) {
	user, err := getUserFromContext(c)
	if err != nil {
		return nil, err
	}

	servers, err := model.ListEnabledMCPServers()
	if err != nil {
		return nil, err
	}

	sort.SliceStable(servers, func(i, j int) bool {
		if servers[i].GetPriority() == servers[j].GetPriority() {
			return servers[i].Id < servers[j].Id
		}
		return servers[i].GetPriority() > servers[j].GetPriority()
	})

	var descriptors []mcp.ToolDescriptor
	for _, server := range servers {
		tools, err := model.GetMCPToolsByServerID(server.Id)
		if err != nil {
			return nil, err
		}
		resolved, err := mcp.ResolveTools(server, tools, nil, user.MCPToolBlacklist, nil)
		if err != nil {
			return nil, err
		}
		for _, entry := range resolved {
			if !entry.Policy.Allowed {
				continue
			}
			var schema map[string]any
			if entry.Tool.InputSchema != "" {
				_ = json.Unmarshal([]byte(entry.Tool.InputSchema), &schema)
			}
			name := server.Name + "." + entry.Tool.Name
			descriptors = append(descriptors, mcp.ToolDescriptor{
				Name:        name,
				Description: entry.Tool.Description,
				InputSchema: schema,
			})
		}
	}
	return descriptors, nil
}

// callMCPToolForUser invokes a MCP tool and applies billing/logging.
func callMCPToolForUser(ctx context.Context, c *gin.Context, params mcpCallParams) (*mcp.CallToolResult, error) {
	logger := gmw.GetLogger(c)
	user, err := getUserFromContext(c)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		servers = []*model.MCPServer{server}
		serverByID[server.Id] = server
	} else {
		servers, err = model.ListEnabledMCPServers()
		if err != nil {
			return nil, err
		}
		for _, server := range servers {
			if server == nil {
				continue
			}
			serverByID[server.Id] = server
		}
	}

	toolsByServer := make(map[int][]*model.MCPTool, len(servers))
	for _, server := range servers {
		if server == nil {
			continue
		}
		tools, err := model.GetMCPToolsByServerID(server.Id)
		if err != nil {
			return nil, err
		}
		toolsByServer[server.Id] = tools
	}

	candidates, err := mcp.BuildToolCandidates(servers, toolsByServer, nil, user.MCPToolBlacklist, []string{toolName}, toolName, params.Signature)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, errors.New("no eligible MCP tool found")
	}

	selected, result, err := mcp.CallWithFallback(ctx, candidates, func(ctx context.Context, candidate mcp.ToolCandidate) (*mcp.CallToolResult, error) {
		server := serverByID[candidate.ServerID]
		if server == nil {
			return nil, errors.New("mcp server not loaded")
		}
		client := mcp.NewStreamableHTTPClientWithLogger(server, nil, time.Duration(config.MCPToolCallTimeoutSec)*time.Second, logger)
		callResult, err := client.CallTool(ctx, candidate.Tool.Name, params.Arguments)
		if err != nil {
			logger.Warn("mcp tool call failed", zap.Error(err), zap.Int("server_id", candidate.ServerID), zap.String("tool", candidate.Tool.Name))
			return nil, err
		}
		return callResult, nil
	})
	if err != nil {
		return nil, err
	}

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
			return nil, err
		}
		model.UpdateUserUsedQuotaAndRequestCount(user.Id, cost)
		qualifiedName := server.Name + "." + selected.Tool.Name
		recordMCPToolLog(ctx, c, user.Id, server.Id, qualifiedName, cost)
	}

	return result, nil
}

// resolveToolCost determines the quota cost for a MCP tool invocation.
func resolveToolCost(server *model.MCPServer, toolName string) int64 {
	pricing := server.ToolPricing[strings.ToLower(toolName)]
	if pricing.QuotaPerCall > 0 {
		return pricing.QuotaPerCall
	}
	if pricing.UsdPerCall > 0 {
		return int64(pricing.UsdPerCall * float64(ratio.QuotaPerUsd))
	}
	return 0
}

// recordMCPToolLog records MCP tool usage into the consume log.
func recordMCPToolLog(ctx context.Context, c *gin.Context, userId int, serverId int, toolName string, cost int64) {
	if cost <= 0 {
		return
	}
	summary := &model.ToolUsageSummary{
		TotalCost:  cost,
		Counts:     map[string]int{toolName: 1},
		CostByTool: map[string]int64{toolName: cost},
		Entries: []model.ToolUsageEntry{
			{
				Tool:     toolName,
				Source:   "oneapi_builtin",
				ServerID: serverId,
				Count:    1,
				Cost:     cost,
			},
		},
	}
	metadata := model.AppendToolUsageMetadata(nil, summary)
	model.RecordConsumeLog(ctx, &model.Log{
		UserId:      userId,
		ModelName:   "mcp",
		Quota:       int(cost),
		Content:     "MCP tool call",
		RequestId:   c.GetString(ctxkey.RequestId),
		TraceId:     tracing.GetTraceID(c),
		Metadata:    metadata,
		IsStream:    false,
		ElapsedTime: helper.CalcElapsedTime(time.Now().Add(-time.Millisecond)),
	})
}

// getUserFromContext loads the authenticated user from request context.
func getUserFromContext(c *gin.Context) (*model.User, error) {
	userID := c.GetInt(ctxkey.Id)
	if userID == 0 {
		return nil, errors.New("user id missing")
	}
	user, err := model.GetUserById(userID, true)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// splitToolName splits server-qualified tool names.
func splitToolName(name string) (string, string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

// isToolAllowed checks if a tool is permitted by the resolved policy.
func isToolAllowed(resolved []mcp.ResolvedTool, name string) bool {
	for _, entry := range resolved {
		if strings.EqualFold(entry.Tool.Name, name) {
			return entry.Policy.Allowed
		}
	}
	return false
}

// respondMCPResult writes a JSON-RPC result payload.
func respondMCPResult(c *gin.Context, id any, result any) {
	c.JSON(http.StatusOK, gin.H{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

// respondMCPError writes a JSON-RPC error payload.
func respondMCPError(c *gin.Context, id any, err error) {
	c.JSON(http.StatusOK, gin.H{
		"jsonrpc": "2.0",
		"id":      id,
		"error": gin.H{
			"message": err.Error(),
		},
	})
}
