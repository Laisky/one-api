package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/mcp"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// claudeToolSearchMCPRegistry holds MCP tool candidates discovered during tool search injection.
type claudeToolSearchMCPRegistry struct {
	candidatesByName map[string][]mcp.ToolCandidate
	requestHeaders   map[string]map[string]string
	selectedIndex    map[string]int
}

// isMCPTool reports whether the registry has candidates for the given tool name.
func (r *claudeToolSearchMCPRegistry) isMCPTool(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.candidatesByName[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// selectedCandidateIndex returns the selected candidate index for a tool.
func (r *claudeToolSearchMCPRegistry) selectedCandidateIndex(name string) int {
	if r == nil || name == "" {
		return 0
	}
	if idx, ok := r.selectedIndex[name]; ok {
		return idx
	}
	return 0
}

// hasToolSearchInClaudeRequest checks if the Claude request contains tool search tools.
func hasToolSearchInClaudeRequest(request *ClaudeMessagesRequest) bool {
	if request == nil {
		return false
	}
	for _, tool := range request.Tools {
		typeName := strings.ToLower(strings.TrimSpace(tool.Type))
		if isToolSearchType(typeName) {
			return true
		}
	}
	return false
}

// isToolSearchType checks if a tool type string is a tool search type.
func isToolSearchType(typeName string) bool {
	return typeName == anthropic.ToolTypeToolSearchRegex ||
		typeName == anthropic.ToolTypeToolSearchBM25 ||
		strings.HasPrefix(typeName, anthropic.ToolTypeToolSearchRegexPrefix) ||
		strings.HasPrefix(typeName, anthropic.ToolTypeToolSearchBM25Prefix)
}

// injectDeferredMCPToolsForToolSearch loads MCP tools from the catalog and injects them
// as deferred Claude tools into the request. Returns a registry for later MCP execution.
func injectDeferredMCPToolsForToolSearch(c *gin.Context, request *ClaudeMessagesRequest) (*claudeToolSearchMCPRegistry, error) {
	if request == nil {
		return nil, nil
	}
	lg := gmw.GetLogger(c)

	user, err := getRelayUserFromContext(c)
	if err != nil {
		return nil, errors.Wrap(err, "get relay user from context")
	}

	channelRecord := func() *model.Channel {
		if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
			if channel, ok := channelModel.(*model.Channel); ok {
				return channel
			}
		}
		return nil
	}()
	channelBlacklist, err := loadChannelMCPBlacklist(channelRecord)
	if err != nil {
		return nil, errors.Wrap(err, "load channel mcp blacklist")
	}

	catalog, err := loadMCPToolCatalog()
	if err != nil {
		return nil, errors.Wrap(err, "load mcp tool catalog")
	}
	if len(catalog.servers) == 0 {
		return nil, nil
	}

	registry := &claudeToolSearchMCPRegistry{
		candidatesByName: make(map[string][]mcp.ToolCandidate),
		requestHeaders:   make(map[string]map[string]string),
		selectedIndex:    make(map[string]int),
	}

	// Collect existing tool names to avoid duplicates
	existingTools := make(map[string]struct{})
	for _, tool := range request.Tools {
		name := strings.ToLower(strings.TrimSpace(tool.Name))
		if name != "" {
			existingTools[name] = struct{}{}
		}
	}

	deferLoading := true
	injectedCount := 0

	for _, server := range catalog.servers {
		if server == nil {
			continue
		}
		tools := catalog.toolsByServer[server.Id]
		if len(tools) == 0 {
			continue
		}
		resolved, err := mcp.ResolveTools(server, tools, channelBlacklist, user.MCPToolBlacklist, nil)
		if err != nil {
			lg.Warn("failed to resolve MCP tools for tool search injection",
				zap.Int("server_id", server.Id),
				zap.String("server_name", server.Name),
				zap.Error(err),
			)
			continue
		}
		for _, entry := range resolved {
			if !entry.Policy.Allowed || entry.Tool == nil {
				continue
			}
			toolName := entry.Tool.Name
			nameKey := strings.ToLower(strings.TrimSpace(toolName))
			if _, exists := existingTools[nameKey]; exists {
				continue
			}

			// Parse input schema
			var inputSchema any
			if entry.Tool.InputSchema != "" {
				var schema map[string]any
				if err := json.Unmarshal([]byte(entry.Tool.InputSchema), &schema); err == nil {
					inputSchema = schema
				}
			}
			if inputSchema == nil {
				inputSchema = map[string]any{"type": "object"}
			}

			// Inject as deferred Claude tool
			request.Tools = append(request.Tools, relaymodel.ClaudeTool{
				Name:         toolName,
				Description:  entry.Tool.Description,
				InputSchema:  inputSchema,
				DeferLoading: &deferLoading,
			})
			existingTools[nameKey] = struct{}{}

			// Build candidates for later execution
			candidates, err := mcp.BuildToolCandidates(
				catalog.servers, catalog.toolsByServer,
				channelBlacklist, user.MCPToolBlacklist,
				[]string{toolName}, toolName, "",
			)
			if err != nil {
				lg.Warn("failed to build MCP tool candidates",
					zap.String("tool", toolName),
					zap.Error(err),
				)
				continue
			}
			if len(candidates) > 0 {
				registry.candidatesByName[nameKey] = candidates
				registry.selectedIndex[nameKey] = 0
			}
			injectedCount++
		}
	}

	if injectedCount == 0 {
		return nil, nil
	}

	lg.Debug("injected deferred MCP tools for tool search",
		zap.Int("count", injectedCount),
		zap.Int("total_tools", len(request.Tools)),
	)
	return registry, nil
}

// executeClaudeToolSearchMCPLoop runs a Claude-native multi-round tool execution loop
// for requests using tool search with MCP tools.
func executeClaudeToolSearchMCPLoop(
	c *gin.Context,
	meta *metalib.Meta,
	request *ClaudeMessagesRequest,
	registry *claudeToolSearchMCPRegistry,
	adaptorInstance adaptor.Adaptor,
	preConsumedQuota int64,
) (*anthropic.Response, *relaymodel.Usage, *mcpExecutionSummary, int64, *relaymodel.ErrorWithStatusCode) {
	if request == nil || registry == nil {
		return nil, nil, nil, 0, nil
	}
	lg := gmw.GetLogger(c)

	maxRounds := config.MCPMaxToolRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}

	channelModelRatio, _ := getChannelRatios(c)
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.GetModelRatioWithThreeLayers(request.Model, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	ratio := modelRatio * groupRatio

	var accumulated *relaymodel.Usage
	var incrementalCharged int64
	executedToolCalls := make(map[string]struct{})
	summary := &mcpExecutionSummary{summary: &model.ToolUsageSummary{
		Counts:     map[string]int{},
		CostByTool: map[string]int64{},
	}}

	for round := 0; round < maxRounds; round++ {
		// Pre-consume quota for this round
		promptTokens := getClaudeMessagesPromptTokens(gmw.Ctx(c), request)
		roundQuota := int64(float64(promptTokens) * ratio)
		if roundQuota > 0 {
			if err := preConsumeQuotaForMCPRound(c, meta, roundQuota); err != nil {
				return nil, accumulated, summary, incrementalCharged,
					openai.ErrorWrapper(err, "pre_consume_tool_search_round_failed", 403)
			}
			incrementalCharged += roundQuota
		}

		// Send request to upstream
		claudeResp, usage, respErr := doClaudeRequestOnce(c, meta, adaptorInstance, request)
		if respErr != nil {
			if roundQuota > 0 {
				billing.ReturnPreConsumedQuota(gmw.Ctx(c), roundQuota, meta.TokenId)
				incrementalCharged -= roundQuota
			}
			return nil, accumulated, summary, incrementalCharged, respErr
		}
		accumulated = mergeUsage(accumulated, usage)

		// Check if response has MCP tool calls
		mcpToolCalls := extractMCPToolCalls(claudeResp, registry)
		if len(mcpToolCalls) == 0 {
			// No MCP tool calls - return response as-is
			return claudeResp, accumulated, summary, incrementalCharged, nil
		}

		lg.Debug("claude tool search MCP tool calls detected",
			zap.Int("round", round+1),
			zap.Int("mcp_tool_calls", len(mcpToolCalls)),
		)

		// Execute MCP tool calls
		toolResults, execErr := executeClaudeMCPToolCalls(c, registry, mcpToolCalls, executedToolCalls, summary)
		if execErr != nil {
			return nil, accumulated, summary, incrementalCharged,
				openai.ErrorWrapper(execErr, "tool_search_mcp_call_failed", 500)
		}

		// Build continuation: add assistant response and tool results to messages
		request.Messages = append(request.Messages, buildClaudeAssistantMessage(claudeResp))
		request.Messages = append(request.Messages, buildClaudeToolResultMessage(claudeResp, toolResults)...)

		lg.Debug("claude tool search MCP round completed", zap.Int("round", round+1))
	}

	return nil, accumulated, summary, incrementalCharged,
		openai.ErrorWrapper(errors.New("tool search MCP tool rounds exceeded"), "tool_search_mcp_rounds_exceeded", 400)
}

// doClaudeRequestOnce sends a single Claude Messages request to upstream and parses the response.
func doClaudeRequestOnce(
	c *gin.Context,
	meta *metalib.Meta,
	adaptorInstance adaptor.Adaptor,
	request *ClaudeMessagesRequest,
) (*anthropic.Response, *relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)

	// Marshal the request
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "marshal_claude_request_failed", 500)
	}

	// Do the upstream request
	resp, err := adaptorInstance.DoRequest(c, meta, bytes.NewReader(requestBytes))
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "do_claude_request_failed", 500)
	}
	if resp == nil {
		return nil, nil, openai.ErrorWrapper(errors.New("nil response from upstream"), "nil_upstream_response", 500)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, RelayErrorHandlerWithContext(c, resp)
	}

	// Read and parse the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "read_claude_response_failed", 500)
	}
	defer resp.Body.Close()

	var claudeResp anthropic.Response
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, nil, openai.ErrorWrapper(err, "parse_claude_response_failed", 500)
	}

	usage := &relaymodel.Usage{
		PromptTokens:     claudeResp.Usage.InputTokens,
		CompletionTokens: claudeResp.Usage.OutputTokens,
		TotalTokens:      claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
	}
	if claudeResp.Usage.CacheReadInputTokens > 0 {
		usage.PromptTokensDetails = &relaymodel.UsagePromptTokensDetails{
			CachedTokens: claudeResp.Usage.CacheReadInputTokens,
		}
	}
	if claudeResp.Usage.CacheCreation != nil {
		usage.CacheWrite5mTokens = claudeResp.Usage.CacheCreation.Ephemeral5mInputTokens
		usage.CacheWrite1hTokens = claudeResp.Usage.CacheCreation.Ephemeral1hInputTokens
	} else if claudeResp.Usage.CacheCreationInputTokens > 0 {
		usage.CacheWrite5mTokens = claudeResp.Usage.CacheCreationInputTokens
	}

	lg.Debug("claude tool search upstream response",
		zap.String("stop_reason", stringPtrValue(claudeResp.StopReason)),
		zap.Int("content_blocks", len(claudeResp.Content)),
		zap.Int("input_tokens", claudeResp.Usage.InputTokens),
		zap.Int("output_tokens", claudeResp.Usage.OutputTokens),
	)

	return &claudeResp, usage, nil
}

// claudeMCPToolCall represents a tool_use block from a Claude response that targets an MCP tool.
type claudeMCPToolCall struct {
	ID    string
	Name  string
	Input map[string]any
}

// extractMCPToolCalls finds tool_use content blocks in the Claude response that reference MCP tools.
func extractMCPToolCalls(resp *anthropic.Response, registry *claudeToolSearchMCPRegistry) []claudeMCPToolCall {
	if resp == nil || registry == nil {
		return nil
	}
	// Only process when Claude stopped for tool use
	if resp.StopReason == nil || *resp.StopReason != "tool_use" {
		return nil
	}

	var calls []claudeMCPToolCall
	for _, block := range resp.Content {
		if block.Type != "tool_use" {
			continue
		}
		if !registry.isMCPTool(block.Name) {
			continue
		}
		args, ok := block.Input.(map[string]any)
		if !ok {
			args = map[string]any{}
		}
		calls = append(calls, claudeMCPToolCall{
			ID:    block.Id,
			Name:  block.Name,
			Input: args,
		})
	}
	return calls
}

// executeClaudeMCPToolCalls invokes MCP tools and returns a map of callID -> result content string.
func executeClaudeMCPToolCalls(
	c *gin.Context,
	registry *claudeToolSearchMCPRegistry,
	calls []claudeMCPToolCall,
	executed map[string]struct{},
	summary *mcpExecutionSummary,
) (map[string]string, error) {
	lg := gmw.GetLogger(c)
	results := make(map[string]string, len(calls))

	// Build a temporary mcpToolRegistry for callMCPToolWithFallback compatibility
	compatRegistry := &mcpToolRegistry{
		candidatesByName: registry.candidatesByName,
		requestHeaders:   registry.requestHeaders,
		selectedIndex:    registry.selectedIndex,
	}

	for _, call := range calls {
		callID := strings.TrimSpace(call.ID)
		if callID != "" {
			if _, exists := executed[callID]; exists {
				continue
			}
			executed[callID] = struct{}{}
		}

		nameKey := strings.ToLower(strings.TrimSpace(call.Name))
		candidates := registry.candidatesByName[nameKey]
		if len(candidates) == 0 {
			results[callID] = `{"error": "no MCP tool candidates available"}`
			continue
		}

		startIndex := registry.selectedCandidateIndex(nameKey)
		selected, result, err := callMCPToolWithFallback(c, compatRegistry, nameKey, call.Input, candidates, startIndex, []string{callID})
		if err != nil {
			results[callID] = `{"error": ` + marshalStringJSON(err.Error()) + `}`
			lg.Warn("MCP tool call failed in tool search loop",
				zap.String("tool", call.Name),
				zap.Error(err),
			)
			continue
		}

		// Format result as string content
		if result == nil {
			results[callID] = ""
		} else if len(result.Raw) > 0 {
			results[callID] = string(result.Raw)
		} else {
			payload := map[string]any{"content": result.Content}
			if result.IsError {
				payload["is_error"] = true
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				results[callID] = `{"error": "failed to marshal tool result"}`
			} else {
				results[callID] = string(encoded)
			}
		}

		recordMCPToolUsage(summary, selected, call.Name)
		lg.Debug("tool search MCP tool call completed",
			zap.String("tool", call.Name),
			zap.Int("server_id", selected.ServerID),
			zap.String("server_label", selected.ServerLabel),
		)
	}

	return results, nil
}

// buildClaudeAssistantMessage converts a Claude API response into a ClaudeMessage for the assistant turn.
func buildClaudeAssistantMessage(resp *anthropic.Response) relaymodel.ClaudeMessage {
	// Re-serialize the content blocks to preserve all fields
	var contentBlocks []any
	for _, block := range resp.Content {
		contentBlock := map[string]any{"type": block.Type}
		switch block.Type {
		case "text":
			contentBlock["text"] = block.Text
		case "tool_use":
			contentBlock["id"] = block.Id
			contentBlock["name"] = block.Name
			contentBlock["input"] = block.Input
		case "thinking":
			if block.Thinking != nil {
				contentBlock["thinking"] = *block.Thinking
			}
			if block.Signature != nil {
				contentBlock["signature"] = *block.Signature
			}
		case "server_tool_use":
			contentBlock["id"] = block.Id
			contentBlock["name"] = block.Name
			contentBlock["input"] = block.Input
		default:
			// Preserve other block types as-is
			contentBlock["text"] = block.Text
			if block.Id != "" {
				contentBlock["id"] = block.Id
			}
			if block.Name != "" {
				contentBlock["name"] = block.Name
			}
			if block.Input != nil {
				contentBlock["input"] = block.Input
			}
		}
		contentBlocks = append(contentBlocks, contentBlock)
	}

	return relaymodel.ClaudeMessage{
		Role:    "assistant",
		Content: contentBlocks,
	}
}

// buildClaudeToolResultMessage creates user messages containing tool_result blocks
// for each MCP tool call that was executed.
func buildClaudeToolResultMessage(resp *anthropic.Response, results map[string]string) []relaymodel.ClaudeMessage {
	if len(results) == 0 {
		return nil
	}

	var toolResults []any
	for _, block := range resp.Content {
		if block.Type != "tool_use" {
			continue
		}
		resultContent, ok := results[block.Id]
		if !ok {
			continue
		}
		toolResults = append(toolResults, map[string]any{
			"type":        "tool_result",
			"tool_use_id": block.Id,
			"content":     resultContent,
		})
	}

	if len(toolResults) == 0 {
		return nil
	}

	return []relaymodel.ClaudeMessage{
		{
			Role:    "user",
			Content: toolResults,
		},
	}
}

// preConsumeQuotaForMCPRound pre-consumes quota for an MCP round in the tool search loop.
func preConsumeQuotaForMCPRound(c *gin.Context, meta *metalib.Meta, quota int64) error {
	if quota <= 0 {
		return nil
	}
	ctx := gmw.Ctx(c)
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return errors.Wrap(err, "get user quota for MCP round")
	}
	if userQuota-quota < 0 {
		return errors.New("user quota is not enough")
	}
	if err := model.PreConsumeTokenQuota(ctx, meta.TokenId, quota); err != nil {
		return errors.Wrap(err, "pre-consume token quota for MCP round")
	}
	syncUserQuotaCacheAfterPreConsume(ctx, meta.UserId, quota, "claude_mcp_round_preconsume")
	return nil
}

// marshalStringJSON marshals a string to JSON format.
func marshalStringJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `"unknown error"`
	}
	return string(b)
}

// stringPtrValue returns the string value of a pointer, or empty string if nil.
func stringPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// writeClaudeResponse writes a Claude API response directly to the HTTP response.
func writeClaudeResponse(c *gin.Context, resp *anthropic.Response) error {
	body, err := json.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshal claude response")
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, err = c.Writer.Write(body)
	return errors.Wrap(err, "write claude response body")
}

// resolveServerByIDFromCatalog looks up a server from the catalog by its ID.
func resolveServerByIDFromCatalog(catalog *mcpToolCatalog, serverID int) *model.MCPServer {
	if catalog == nil {
		return nil
	}
	for _, server := range catalog.servers {
		if server != nil && server.Id == serverID {
			return server
		}
	}
	return nil
}
