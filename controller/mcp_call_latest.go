package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

type modernMCPToolCallPlan struct {
	userID     int
	serverByID map[int]*model.MCPServer
	candidates []mcp.ToolCandidate
}

// executeModernMCPTool validates one modern call against its lossless descriptor and executes it.
//
// Parameters:
//   - ctx: The request context controls database and upstream work.
//   - c: The Gin context contains the authenticated user and request headers.
//   - params: The call parameters contain the exact qualified tool name, arguments, signature, and optional MRTR state.
//
// Return values:
//   - *mcp.CallToolResult: The normalized upstream result is returned on success.
//   - error: A modern validation error or a wrapped routing, execution, or billing error is returned on failure.
func executeModernMCPTool(ctx context.Context, c *gin.Context, params modernMCPCallParams) (*mcp.CallToolResult, error) {
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}

	plan, err := prepareModernMCPToolCall(c, params)
	if err != nil {
		return nil, &modernMCPValidationError{
			Status: http.StatusOK,
			Code:   mcpErrInvalidParams,
			Err:    errors.Wrap(err, "resolve modern mcp tool descriptor"),
		}
	}
	descriptor, err := resolveModernMCPToolDescriptor(ctx, c, params.Name, plan)
	if err != nil {
		return nil, &modernMCPValidationError{
			Status: http.StatusOK,
			Code:   mcpErrInvalidParams,
			Err:    errors.Wrap(err, "resolve modern mcp tool descriptor"),
		}
	}
	if err := mcp.ValidateToolArgumentHeaders(c.Request.Header, descriptor.InputSchema, params.Arguments); err != nil {
		return nil, &modernMCPValidationError{
			Status: http.StatusBadRequest,
			Code:   mcp.ErrorCodeHeaderMismatch,
			Err:    err,
		}
	}

	result, err := executeModernMCPToolCall(ctx, c, params, plan)
	if err != nil {
		return nil, err
	}
	return mcp.NormalizeCallToolResult(result), nil
}

// prepareModernMCPToolCall loads and resolves one immutable candidate snapshot for validation and execution.
//
// Parameters:
//   - c: The Gin context contains the authenticated user.
//   - params: The call parameters contain the exact qualified tool name and optional signature.
//
// Return values:
//   - *modernMCPToolCallPlan: The prepared user, server, and candidate snapshot.
//   - error: A wrapped authentication, catalog, policy, or candidate error.
func prepareModernMCPToolCall(c *gin.Context, params modernMCPCallParams) (*modernMCPToolCallPlan, error) {
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

	return &modernMCPToolCallPlan{
		userID:     user.Id,
		serverByID: serverByID,
		candidates: candidates,
	}, nil
}

// resolveModernMCPToolDescriptor restores the descriptor from the prepared candidate snapshot.
//
// Parameters:
//   - ctx: The request context is used only by the compatibility fallback.
//   - c: The Gin context is used only by the compatibility fallback.
//   - name: The exact qualified wire name returned by tools/list.
//   - plan: The prepared candidate snapshot; nil retains the catalog lookup fallback for isolated callers.
//
// Return values:
//   - mcp.ToolDescriptor: The exact policy-filtered descriptor used by execution.
//   - error: A wrapped descriptor or unknown-tool error.
func resolveModernMCPToolDescriptor(
	ctx context.Context,
	c *gin.Context,
	name string,
	plan *modernMCPToolCallPlan,
) (mcp.ToolDescriptor, error) {
	if plan == nil {
		return findModernMCPToolDescriptor(ctx, c, name)
	}
	for _, candidate := range plan.candidates {
		if candidate.Tool == nil {
			continue
		}
		qualifiedName := candidate.ServerLabel + "." + strings.TrimSpace(candidate.Tool.Name)
		if qualifiedName != name {
			continue
		}
		descriptor, err := descriptorForMCPTool(candidate.Tool)
		if err != nil {
			return mcp.ToolDescriptor{}, errors.Wrapf(
				err,
				"restore mcp descriptor for server %d tool %q",
				candidate.ServerID,
				candidate.Tool.Name,
			)
		}
		descriptor.Name = qualifiedName
		return descriptor, nil
	}
	return mcp.ToolDescriptor{}, errors.Errorf("no eligible MCP tool found for %q", name)
}

// callMCPToolForUserLatest routes one exact tool name across eligible servers and applies final-result billing.
//
// Parameters:
//   - ctx: The request context controls database and upstream work.
//   - c: The Gin context contains the authenticated user and request-scoped logger.
//   - params: The call parameters contain the exact qualified tool name, arguments, signature, and optional MRTR state.
//
// Return values:
//   - *mcp.CallToolResult: The normalized upstream result, including input_required intermediates, is returned on success.
//   - error: A wrapped authentication, catalog, routing, execution, or billing error is returned on failure.
func callMCPToolForUserLatest(ctx context.Context, c *gin.Context, params modernMCPCallParams) (*mcp.CallToolResult, error) {
	plan, err := prepareModernMCPToolCall(c, params)
	if err != nil {
		return nil, err
	}
	return executeModernMCPToolCall(ctx, c, params, plan)
}

// executeModernMCPToolCall executes one prepared modern tool call with fallback and final-result billing.
//
// Parameters:
//   - ctx: The request context controls upstream work.
//   - c: The Gin context contains the request-scoped logger.
//   - params: The call parameters contain arguments and optional MRTR state.
//   - plan: The immutable user, server, and candidate snapshot prepared for this request.
//
// Return values:
//   - *mcp.CallToolResult: The normalized upstream result, including input_required intermediates, is returned on success.
//   - error: A wrapped routing, execution, or billing error is returned on failure.
func executeModernMCPToolCall(
	ctx context.Context,
	c *gin.Context,
	params modernMCPCallParams,
	plan *modernMCPToolCallPlan,
) (*mcp.CallToolResult, error) {
	if plan == nil {
		return nil, errors.WithStack(errors.New("modern mcp tool call plan is nil"))
	}

	logger := gmw.GetLogger(c)
	startedAt := time.Now() // Preserve the monotonic component for elapsed-time measurement.
	selected, result, err := mcp.CallWithFallback(ctx, plan.candidates, func(ctx context.Context, candidate mcp.ToolCandidate) (*mcp.CallToolResult, error) {
		server := plan.serverByID[candidate.ServerID]
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
	if err := chargeAndRecordMCPToolCall(ctx, c, plan.userID, plan.serverByID, selected, startedAt); err != nil {
		return nil, err
	}
	return result, nil
}

// filterExactMCPToolCandidates removes candidates that only matched case-insensitive policy normalization.
//
// Parameters:
//   - candidates: The policy-filtered candidates come from the shared registry.
//   - exactName: The exactName value is the case-sensitive upstream wire name requested by the client.
//
// Return values:
//   - []mcp.ToolCandidate: The returned candidates have stored wire names that exactly equal exactName.
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
//   - result: The normalized upstream result is inspected for final completion.
//
// Return values:
//   - bool: True is returned only for non-error results whose resultType is complete.
func shouldBillMCPToolResult(result *mcp.CallToolResult) bool {
	return result != nil && !result.IsError && result.ResultType == mcp.ResultTypeComplete
}
