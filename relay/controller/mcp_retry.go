package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/mcp"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// mcpToolSchemaMismatchError signals that tool arguments do not satisfy a fallback tool schema.
type mcpToolSchemaMismatchError struct {
	ToolName       string
	CandidateIndex int
	CallIDs        []string
	Reason         string
}

// Error returns the error message.
func (e *mcpToolSchemaMismatchError) Error() string {
	if e == nil {
		return "mcp tool schema mismatch"
	}
	return fmt.Sprintf("mcp tool schema mismatch for %s: %s", e.ToolName, e.Reason)
}

// selectedCandidateIndex returns the selected candidate index for a tool.
func (r *mcpToolRegistry) selectedCandidateIndex(name string) int {
	if r == nil || name == "" {
		return 0
	}
	if idx, ok := r.selectedIndex[name]; ok {
		return idx
	}
	return 0
}

// setSelectedCandidate updates the selected candidate index and reports if it changed.
func (r *mcpToolRegistry) setSelectedCandidate(name string, index int) bool {
	if r == nil || name == "" {
		return false
	}
	if index < 0 {
		index = 0
	}
	current, ok := r.selectedIndex[name]
	if ok && current == index {
		return false
	}
	r.selectedIndex[name] = index
	return true
}

// rebuildRequestTools rebuilds request tool definitions using the selected MCP candidates.
func (r *mcpToolRegistry) rebuildRequestTools(request *relaymodel.GeneralOpenAIRequest) error {
	if r == nil || request == nil {
		return errors.New("mcp registry or request is nil")
	}
	if len(r.originalTools) == 0 {
		return nil
	}
	updated := make([]relaymodel.Tool, 0, len(r.originalTools))
	for _, tool := range r.originalTools {
		typeKey := strings.ToLower(strings.TrimSpace(tool.Type))
		if typeKey == "" || typeKey == "function" || typeKey == "mcp" {
			updated = append(updated, tool)
			continue
		}
		name := r.toolNameByType[typeKey]
		if name == "" {
			updated = append(updated, tool)
			continue
		}
		candidates := r.candidatesByName[name]
		if len(candidates) == 0 {
			updated = append(updated, tool)
			continue
		}
		idx := r.selectedCandidateIndex(name)
		if idx < 0 || idx >= len(candidates) {
			idx = 0
		}
		functionTool, err := buildFunctionToolFromMCP(candidates[idx])
		if err != nil {
			return err
		}
		updated = append(updated, functionTool)
	}
	request.Tools = updated
	return nil
}

// callMCPToolWithFallback executes MCP tool calls with schema-aware fallback handling.
func callMCPToolWithFallback(c *gin.Context, registry *mcpToolRegistry, nameKey string, args map[string]any, candidates []mcp.ToolCandidate, startIndex int, callIDs []string) (mcp.ToolCandidate, *mcp.CallToolResult, error) {
	if len(candidates) == 0 {
		return mcp.ToolCandidate{}, nil, errors.New("no mcp tool candidates available")
	}
	if startIndex < 0 || startIndex >= len(candidates) {
		startIndex = 0
	}
	lg := gmw.GetLogger(c)
	var lastErr error
	for idx := startIndex; idx < len(candidates); idx++ {
		candidate := candidates[idx]
		if candidate.Tool == nil {
			continue
		}
		if idx == startIndex {
			match, matchErr := validateMCPToolArguments(args, candidate)
			if matchErr != nil {
				return mcp.ToolCandidate{}, nil, errors.Wrap(matchErr, "validate mcp tool arguments")
			}
			if !match {
				lg.Debug("mcp tool arguments do not satisfy selected schema",
					zap.String("tool", nameKey),
					zap.Int("server_id", candidate.ServerID),
					zap.String("server_label", candidate.ServerLabel),
					zap.Strings("argument_keys", argumentKeys(args)),
					zap.String("schema_signature", candidate.Signature),
				)
			}
		} else {
			match, matchErr := validateMCPToolArguments(args, candidate)
			if matchErr != nil {
				return mcp.ToolCandidate{}, nil, errors.Wrap(matchErr, "validate fallback mcp tool arguments")
			}
			if !match {
				lg.Debug("mcp fallback candidate schema mismatch",
					zap.String("tool", nameKey),
					zap.Int("candidate_index", idx),
					zap.Int("server_id", candidate.ServerID),
					zap.String("server_label", candidate.ServerLabel),
					zap.Strings("argument_keys", argumentKeys(args)),
					zap.String("schema_signature", candidate.Signature),
				)
				return mcp.ToolCandidate{}, nil, errors.WithStack(&mcpToolSchemaMismatchError{
					ToolName:       nameKey,
					CandidateIndex: idx,
					CallIDs:        filterCallIDs(callIDs),
					Reason:         "arguments do not satisfy schema",
				})
			}
		}

		selected, result, err := invokeMCPTool(c, registry, nameKey, candidate, args)
		if err == nil {
			return selected, result, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			break
		}
	}
	return mcp.ToolCandidate{}, nil, errors.Wrap(lastErr, "mcp tool call failed after retries")
}

// invokeMCPTool sends an MCP tool call to the selected server.
func invokeMCPTool(c *gin.Context, registry *mcpToolRegistry, nameKey string, candidate mcp.ToolCandidate, args map[string]any) (mcp.ToolCandidate, *mcp.CallToolResult, error) {
	server := resolveServerByID(candidate.ServerID)
	if server == nil {
		return mcp.ToolCandidate{}, nil, errors.New("mcp server not loaded")
	}
	headers := registry.requestHeaders[nameKey]
	lg := gmw.GetLogger(c)
	client := mcp.NewStreamableHTTPClientWithLogger(server, headers, time.Duration(config.MCPToolCallTimeoutSec)*time.Second, lg)
	lg.Debug("invoking mcp tool",
		zap.String("tool", candidate.Tool.Name),
		zap.Int("server_id", candidate.ServerID),
		zap.String("server_label", candidate.ServerLabel),
	)
	result, err := client.CallTool(gmw.Ctx(c), candidate.Tool.Name, args)
	if err != nil {
		return mcp.ToolCandidate{}, nil, err
	}
	return candidate, result, nil
}

// validateMCPToolArguments validates tool arguments against the candidate schema.
func validateMCPToolArguments(args map[string]any, candidate mcp.ToolCandidate) (bool, error) {
	if candidate.Tool == nil {
		return false, errors.New("mcp tool is nil")
	}
	schema, err := mcp.ParseInputSchema(candidate.Tool.InputSchema)
	if err != nil {
		return false, err
	}
	match, err := mcp.ArgumentsMatchSchema(args, schema)
	if err != nil {
		return false, err
	}
	return match, nil
}

// argumentKeys returns sorted argument keys for logging.
func argumentKeys(args map[string]any) []string {
	if len(args) == 0 {
		return nil
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// filterCallIDs removes empty call IDs.
func filterCallIDs(ids []string) []string {
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}
