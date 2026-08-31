package controller

import (
	"context"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// executeModernMCPTool validates one modern call against its lossless descriptor and executes it.
//
// Parameters:
//   - ctx: the request context controlling database and upstream work.
//   - c: the Gin context containing the authenticated user and request headers.
//   - params: the exact qualified tool name, arguments, signature, and optional MRTR state.
//
// Return values:
//   - *mcp.CallToolResult: the normalized upstream result.
//   - error: a wrapped descriptor, header, routing, execution, or billing error.
func executeModernMCPTool(ctx context.Context, c *gin.Context, params modernMCPCallParams) (*mcp.CallToolResult, error) {
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	descriptor, err := findModernMCPToolDescriptor(ctx, c, params.Name)
	if err != nil {
		return nil, errors.Wrap(err, "resolve modern mcp tool descriptor")
	}
	if err := mcp.ValidateToolArgumentHeaders(c.Request.Header, descriptor.InputSchema, params.Arguments); err != nil {
		return nil, &modernMCPValidationError{Status: 400, Code: mcp.ErrorCodeHeaderMismatch, Err: err}
	}
	result, err := callMCPToolForUserLatest(ctx, c, params)
	if err != nil {
		return nil, err
	}
	return mcp.NormalizeCallToolResult(result), nil
}

// callMCPToolForUserLatest routes one exact tool name across eligible servers and applies final-result billing.
//
// Parameters:
//   - ctx: the request context controlling database and upstream work.
//   - c: the Gin context containing the authenticated user and request-scoped logger.
//   - params: the exact qualified tool name, arguments, signature, and optional MRTR state.
//
// Return values:
//   - *mcp.CallToolResult: the normalized upstream result, including input_required intermediates.
//   - error: a wrapped authentication, catalog, routing, execution, or billing error.
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

// filterExactMCPToolCandidates removes candidates that only matched case-insensitive policy normalization.
//
// Parameters:
//   - candidates: policy-filtered candidates returned by the shared registry.
//   - exactName: the exact case-sensitive upstream wire name requested by the client.
//
// Return values:
//   - []mcp.ToolCandidate: candidates whose stored wire names exactly equal exactName.
func filterExactMCPToolCandidates(candidates []mcp.ToolCandidate, exactName string) []mcp.ToolCandidate {
	filtered := make([]mcp.ToolCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Tool == nil || strings.TrimSpace(candidate.Tool.Name) != exactName {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

// shouldBillMCPToolResult reports whether one result represents a successful completed logical call.
//
// Parameters:
//   - result: the normalized upstream result.
//
// Return values:
//   - bool: true only for non-error results whose resultType is complete.
func shouldBillMCPToolResult(result *mcp.CallToolResult) bool {
	return result != nil && !result.IsError && result.ResultType == mcp.ResultTypeComplete
}
