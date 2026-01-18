package controller

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/mcp"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
	"github.com/songquanpeng/one-api/relay/tooling"
)

const (
	mcpToolSourceOneAPI = "oneapi_builtin"
)

type mcpToolRegistry struct {
	candidatesByName map[string][]mcp.ToolCandidate
	requestHeaders   map[string]map[string]string
}

// isMCPTool reports whether the registry has a candidate list for the tool name.
func (r *mcpToolRegistry) isMCPTool(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.candidatesByName[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

type mcpExecutionSummary struct {
	summary *model.ToolUsageSummary
}

// hasMCPBuiltinsInResponseRequest determines whether a Response API request references MCP tools.
func hasMCPBuiltinsInResponseRequest(c *gin.Context, meta *metalib.Meta, channelRecord *model.Channel, provider adaptor.Adaptor, request *openai.ResponseAPIRequest) (bool, error) {
	if request == nil {
		return false, nil
	}
	chatRequest := &relaymodel.GeneralOpenAIRequest{Model: request.Model, Tools: responseToolsForMCP(request)}
	registry, _, err := expandMCPBuiltinsInChatRequest(c, meta, channelRecord, provider, chatRequest)
	if err != nil {
		return false, err
	}
	return registry != nil, nil
}

// responseToolsForMCP extracts non-function tools for MCP matching from a Response API request.
func responseToolsForMCP(request *openai.ResponseAPIRequest) []relaymodel.Tool {
	if request == nil {
		return nil
	}
	tools := make([]relaymodel.Tool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		toolType := strings.TrimSpace(tool.Type)
		if toolType == "" || strings.EqualFold(toolType, "function") {
			continue
		}
		tools = append(tools, relaymodel.Tool{Type: toolType})
	}
	return tools
}

type mcpToolCatalog struct {
	servers       []*model.MCPServer
	toolsByServer map[int][]*model.MCPTool
	serverByLabel map[string]*model.MCPServer
}

// loadMCPToolCatalog loads enabled servers and their tool lists.
func loadMCPToolCatalog() (*mcpToolCatalog, error) {
	servers, err := model.ListEnabledMCPServers()
	if err != nil {
		return nil, err
	}
	catalog := &mcpToolCatalog{
		servers:       servers,
		toolsByServer: make(map[int][]*model.MCPTool, len(servers)),
		serverByLabel: make(map[string]*model.MCPServer, len(servers)),
	}
	for _, server := range servers {
		if server == nil {
			continue
		}
		catalog.serverByLabel[strings.ToLower(server.Name)] = server
		tools, err := model.GetMCPToolsByServerID(server.Id)
		if err != nil {
			return nil, err
		}
		catalog.toolsByServer[server.Id] = tools
	}
	return catalog, nil
}

// loadChannelMCPBlacklist returns the channel MCP tool blacklist from config.
func loadChannelMCPBlacklist(channelRecord *model.Channel) ([]string, error) {
	if channelRecord == nil {
		return nil, nil
	}
	cfg, err := channelRecord.LoadConfig()
	if err != nil {
		return nil, err
	}
	return cfg.MCPToolBlacklist, nil
}

// expandMCPBuiltinsInChatRequest replaces MCP built-ins with local function tools for upstream.
func expandMCPBuiltinsInChatRequest(c *gin.Context, meta *metalib.Meta, channelRecord *model.Channel, provider adaptor.Adaptor, request *relaymodel.GeneralOpenAIRequest) (*mcpToolRegistry, map[string]struct{}, error) {
	if request == nil {
		return nil, nil, nil
	}
	lg := gmw.GetLogger(c)
	user, err := getRelayUserFromContext(c)
	if err != nil {
		return nil, nil, err
	}
	channelBlacklist, err := loadChannelMCPBlacklist(channelRecord)
	if err != nil {
		return nil, nil, err
	}
	catalog, err := loadMCPToolCatalog()
	if err != nil {
		return nil, nil, err
	}

	registry := &mcpToolRegistry{
		candidatesByName: make(map[string][]mcp.ToolCandidate),
		requestHeaders:   make(map[string]map[string]string),
	}
	mcpNames := make(map[string]struct{})
	updatedTools := make([]relaymodel.Tool, 0, len(request.Tools))

	for _, tool := range request.Tools {
		switch strings.ToLower(strings.TrimSpace(tool.Type)) {
		case "function", "":
			if tool.Function != nil {
				updatedTools = append(updatedTools, tool)
				continue
			}
		case "mcp":
			return nil, nil, errors.New("explicit mcp tool definitions are not supported; use standard built-in tool types instead")
		default:
			requested, functionTool, matched, err := expandAliasedMCPTool(catalog, channelBlacklist, user.MCPToolBlacklist, tool.Type)
			if err != nil {
				return nil, nil, err
			}
			if matched {
				name := strings.ToLower(requested.Name)
				mcpNames[name] = struct{}{}
				if builtin := tooling.NormalizeBuiltinType(tool.Type); builtin != "" {
					mcpNames[builtin] = struct{}{}
				}
				registry.candidatesByName[name] = requested.Candidates
				updatedTools = append(updatedTools, functionTool)
				paramKeys := []string{}
				if functionTool.Function != nil {
					if params, ok := functionTool.Function.Parameters.(map[string]any); ok {
						for key := range params {
							paramKeys = append(paramKeys, key)
						}
					}
				}
				lg.Debug("converted tool to mcp",
					zap.String("tool", tool.Type),
					zap.Bool("has_parameters", functionTool.Function != nil && functionTool.Function.Parameters != nil),
					zap.Strings("parameter_keys", paramKeys),
				)
				continue
			}
			if tooling.NormalizeBuiltinType(tool.Type) != "" {
				updatedTools = append(updatedTools, tool)
				continue
			}
		}

		updatedTools = append(updatedTools, tool)
	}

	if len(registry.candidatesByName) == 0 {
		return nil, mcpNames, nil
	}

	request.Tools = updatedTools
	return registry, mcpNames, nil
}

type mcpResolvedRequest struct {
	Name       string
	Candidates []mcp.ToolCandidate
	Headers    map[string]string
}

// expandExplicitMCPTool expands a type=mcp tool into per-tool function tools.
func expandExplicitMCPTool(catalog *mcpToolCatalog, channelBlacklist []string, userBlacklist []string, tool relaymodel.Tool) ([]mcpResolvedRequest, []relaymodel.Tool, error) {
	if tool.ServerLabel == "" {
		return nil, nil, errors.New("mcp tool requires server_label")
	}
	server := catalog.serverByLabel[strings.ToLower(tool.ServerLabel)]
	if server == nil {
		return nil, nil, errors.Errorf("mcp server %s not found", tool.ServerLabel)
	}
	allowed := tool.AllowedTools
	if len(allowed) == 0 {
		resolvedTools, err := mcp.ResolveTools(server, catalog.toolsByServer[server.Id], channelBlacklist, userBlacklist, nil)
		if err != nil {
			return nil, nil, err
		}
		resolved := make([]mcpResolvedRequest, 0, len(resolvedTools))
		functionTools := make([]relaymodel.Tool, 0, len(resolvedTools))
		for _, entry := range resolvedTools {
			if !entry.Policy.Allowed || entry.Tool == nil {
				continue
			}
			candidates, err := mcp.BuildToolCandidates([]*model.MCPServer{server}, catalog.toolsByServer, channelBlacklist, userBlacklist, []string{entry.Tool.Name}, entry.Tool.Name, "")
			if err != nil {
				return nil, nil, err
			}
			if len(candidates) == 0 {
				continue
			}
			functionTool, err := buildFunctionToolFromMCP(candidates[0])
			if err != nil {
				return nil, nil, err
			}
			resolved = append(resolved, mcpResolvedRequest{Name: entry.Tool.Name, Candidates: candidates, Headers: tool.Headers})
			functionTools = append(functionTools, functionTool)
		}
		if len(functionTools) == 0 {
			return nil, nil, errors.New("no eligible MCP tools found")
		}
		return resolved, functionTools, nil
	}

	resolved := make([]mcpResolvedRequest, 0, len(allowed))
	functionTools := make([]relaymodel.Tool, 0, len(allowed))

	for _, name := range allowed {
		candidates, err := mcp.BuildToolCandidates([]*model.MCPServer{server}, catalog.toolsByServer, channelBlacklist, userBlacklist, []string{name}, name, "")
		if err != nil {
			return nil, nil, err
		}
		if len(candidates) == 0 {
			return nil, nil, errors.Errorf("no eligible MCP tool found for %s", name)
		}
		functionTool, err := buildFunctionToolFromMCP(candidates[0])
		if err != nil {
			return nil, nil, err
		}
		resolved = append(resolved, mcpResolvedRequest{Name: name, Candidates: candidates, Headers: tool.Headers})
		functionTools = append(functionTools, functionTool)
	}

	return resolved, functionTools, nil
}

// expandAliasedMCPTool resolves tool type aliases to MCP tools when available.
func expandAliasedMCPTool(catalog *mcpToolCatalog, channelBlacklist []string, userBlacklist []string, toolType string) (mcpResolvedRequest, relaymodel.Tool, bool, error) {
	serverLabel, toolName := splitMCPToolName(toolType)
	name := toolName
	if name == "" {
		name = strings.TrimSpace(toolType)
	}
	if name == "" {
		return mcpResolvedRequest{}, relaymodel.Tool{}, false, nil
	}

	servers := catalog.servers
	if serverLabel != "" {
		server := catalog.serverByLabel[strings.ToLower(serverLabel)]
		if server == nil {
			return mcpResolvedRequest{}, relaymodel.Tool{}, false, nil
		}
		servers = []*model.MCPServer{server}
	}

	candidates, err := mcp.BuildToolCandidates(servers, catalog.toolsByServer, channelBlacklist, userBlacklist, []string{name}, name, "")
	if err != nil {
		return mcpResolvedRequest{}, relaymodel.Tool{}, false, err
	}
	if len(candidates) == 0 {
		return mcpResolvedRequest{}, relaymodel.Tool{}, false, nil
	}
	functionTool, err := buildFunctionToolFromMCP(candidates[0])
	if err != nil {
		return mcpResolvedRequest{}, relaymodel.Tool{}, false, err
	}
	return mcpResolvedRequest{Name: name, Candidates: candidates}, functionTool, true, nil
}

// buildFunctionToolFromMCP converts an MCP tool schema into a local function tool definition.
func buildFunctionToolFromMCP(candidate mcp.ToolCandidate) (relaymodel.Tool, error) {
	if candidate.Tool == nil {
		return relaymodel.Tool{}, errors.New("mcp tool is nil")
	}
	var schema map[string]any
	if candidate.Tool.InputSchema != "" {
		if err := json.Unmarshal([]byte(candidate.Tool.InputSchema), &schema); err != nil {
			return relaymodel.Tool{}, errors.Wrap(err, "parse mcp tool schema")
		}
	}
	if len(schema) == 0 {
		schema = map[string]any{"type": "object"}
	}
	return relaymodel.Tool{
		Type: "function",
		Function: &relaymodel.Function{
			Name:        candidate.Tool.Name,
			Description: candidate.Tool.Description,
			Parameters:  schema,
		},
	}, nil
}

// normalizeChatToolChoiceForMCP coerces tool_choice to function when it targets an MCP alias.
func normalizeChatToolChoiceForMCP(choice any, mcpNames map[string]struct{}) any {
	if choice == nil || len(mcpNames) == 0 {
		return choice
	}
	mapChoice, ok := choice.(map[string]any)
	if !ok {
		return choice
	}
	choiceType, _ := mapChoice["type"].(string)
	name, _ := mapChoice["name"].(string)
	if name == "" {
		name = strings.TrimSpace(choiceType)
	}
	if name == "" {
		return choice
	}
	if _, exists := mcpNames[strings.ToLower(name)]; !exists {
		return choice
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": name,
		},
	}
}

// executeChatMCPToolLoop runs a multi-round tool execution loop for MCP tools.
func executeChatMCPToolLoop(c *gin.Context, meta *metalib.Meta, request *relaymodel.GeneralOpenAIRequest, registry *mcpToolRegistry, basePreConsumedQuota int64) (*openai.TextResponse, *relaymodel.Usage, *mcpExecutionSummary, int64, *relaymodel.ErrorWithStatusCode) {
	if request == nil || registry == nil {
		return nil, nil, nil, 0, nil
	}
	lg := gmw.GetLogger(c)
	adaptorInstance := relay.GetAdaptor(meta.APIType)
	if adaptorInstance == nil {
		return nil, nil, nil, 0, openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", 400)
	}
	adaptorInstance.Init(meta)

	maxRounds := config.MCPMaxToolRounds
	if maxRounds <= 0 {
		maxRounds = 5
	}

	channelModelRatio, channelCompletionRatio := getChannelRatios(c)
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	modelRatio := pricing.GetModelRatioWithThreeLayers(request.Model, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	ratio := modelRatio * groupRatio

	var accumulated *relaymodel.Usage
	var incrementalCharged int64
	executedToolCalls := make(map[string]struct{})
	summary := &mcpExecutionSummary{summary: &model.ToolUsageSummary{Counts: map[string]int{}, CostByTool: map[string]int64{}}}

	for round := 0; round < maxRounds; round++ {
		promptTokens := getPromptTokens(gmw.Ctx(c), request, meta.Mode)
		roundQuota, err := preConsumeMCPRoundQuota(c, meta, request, promptTokens, ratio)
		if err != nil {
			return nil, accumulated, summary, incrementalCharged, openai.ErrorWrapper(err, "pre_consume_mcp_round_failed", 403)
		}
		if roundQuota > 0 {
			incrementalCharged += roundQuota
			updateMCPRequestCostProvisional(c, meta, basePreConsumedQuota+incrementalCharged)
		}

		response, usage, respErr := doChatRequestOnce(c, meta, adaptorInstance, request)
		if respErr != nil {
			if roundQuota > 0 {
				billing.ReturnPreConsumedQuota(gmw.Ctx(c), roundQuota, meta.TokenId)
				incrementalCharged -= roundQuota
				updateMCPRequestCostProvisional(c, meta, basePreConsumedQuota+incrementalCharged)
			}
			return nil, accumulated, summary, incrementalCharged, respErr
		}
		accumulated = mergeUsage(accumulated, usage)
		updateMCPRequestCostEstimate(c, meta, accumulated, request.Model, modelRatio, groupRatio, channelCompletionRatio, pricingAdaptor)

		choice, ok := firstChoice(response)
		if !ok || len(choice.Message.ToolCalls) == 0 {
			return response, accumulated, summary, incrementalCharged, nil
		}

		callNames := make([]string, 0, len(choice.Message.ToolCalls))
		unmatched := make([]string, 0)
		for _, call := range choice.Message.ToolCalls {
			if call.Function == nil {
				unmatched = append(unmatched, "<nil>")
				continue
			}
			callNames = append(callNames, call.Function.Name)
			if !registry.isMCPTool(call.Function.Name) {
				unmatched = append(unmatched, call.Function.Name)
			}
		}
		lg.Debug("mcp tool calls received", zap.Strings("tool_calls", callNames), zap.Strings("unmatched_tools", unmatched))

		if !allMCPToolCalls(choice.Message.ToolCalls, registry) {
			lg.Debug("skipping mcp execution due to non-mcp tool calls")
			return response, accumulated, summary, incrementalCharged, nil
		}

		request.Messages = append(request.Messages, choice.Message)
		previousCost := resolveMCPToolCostSnapshot(summary)
		results, execErr := executeMCPToolCalls(c, registry, choice.Message.ToolCalls, executedToolCalls, summary)
		if execErr != nil {
			return nil, accumulated, summary, incrementalCharged, openai.ErrorWrapper(execErr, "mcp_tool_call_failed", 500)
		}
		accumulated = applyMCPToolCostDelta(accumulated, previousCost, summary)
		updateMCPRequestCostEstimate(c, meta, accumulated, request.Model, modelRatio, groupRatio, channelCompletionRatio, pricingAdaptor)
		if len(results) == 0 {
			return response, accumulated, summary, incrementalCharged, nil
		}
		request.Messages = append(request.Messages, results...)
		lg.Debug("mcp tool round completed", zap.Int("round", round+1))
	}

	return nil, accumulated, summary, incrementalCharged, openai.ErrorWrapper(errors.New("mcp tool rounds exceeded"), "mcp_tool_rounds_exceeded", 400)
}

// doChatRequestOnce executes one upstream chat request and captures the response.
func doChatRequestOnce(c *gin.Context, meta *metalib.Meta, adaptorInstance adaptor.Adaptor, request *relaymodel.GeneralOpenAIRequest) (*openai.TextResponse, *relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
	logMCPRequestToolSchemas(c, request)
	convertedRequest, err := adaptorInstance.ConvertRequest(c, meta.Mode, request)
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "convert_request_failed", 500)
	}
	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "marshal_converted_request_failed", 500)
	}
	resp, err := adaptorInstance.DoRequest(c, meta, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "do_request_failed", 500)
	}
	if isErrorHappened(meta, resp) {
		return nil, nil, RelayErrorHandlerWithContext(c, resp)
	}

	origWriter := c.Writer
	capture := newResponseCaptureWriter(origWriter)
	c.Writer = capture
	c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
	usage, respErr := adaptorInstance.DoResponse(c, resp, meta)
	c.Writer = origWriter
	if respErr != nil && usage == nil {
		return nil, nil, respErr
	}

	var parsed openai.TextResponse
	if err := json.Unmarshal(capture.BodyBytes(), &parsed); err != nil {
		return nil, usage, openai.ErrorWrapper(err, "parse_chat_response_failed", 500)
	}
	if len(parsed.Choices) > 0 {
		choice := parsed.Choices[0]
		toolNames := make([]string, 0, len(choice.Message.ToolCalls))
		for _, call := range choice.Message.ToolCalls {
			if call.Function == nil {
				toolNames = append(toolNames, "<nil>")
				continue
			}
			toolNames = append(toolNames, call.Function.Name)
		}
		lg := gmw.GetLogger(c)
		lg.Debug("upstream tool calls parsed", zap.Strings("tool_calls", toolNames))
	}
	return &parsed, usage, nil
}

// logMCPRequestToolSchemas records MCP tool schema presence before upstream dispatch.
func logMCPRequestToolSchemas(c *gin.Context, request *relaymodel.GeneralOpenAIRequest) {
	if c == nil || request == nil || len(request.Tools) == 0 {
		return
	}
	lg := gmw.GetLogger(c)
	entries := make([]zap.Field, 0, len(request.Tools))
	for idx, tool := range request.Tools {
		if tool.Function == nil {
			entries = append(entries, zap.Bool("tool_"+strconv.Itoa(idx)+"_has_function", false))
			entries = append(entries, zap.String("tool_"+strconv.Itoa(idx)+"_type", tool.Type))
			continue
		}
		params, _ := tool.Function.Parameters.(map[string]any)
		paramKeys := make([]string, 0, len(params))
		for key := range params {
			paramKeys = append(paramKeys, key)
		}
		entries = append(entries,
			zap.String("tool_"+strconv.Itoa(idx)+"_type", tool.Type),
			zap.String("tool_"+strconv.Itoa(idx)+"_name", tool.Function.Name),
			zap.Bool("tool_"+strconv.Itoa(idx)+"_has_parameters", tool.Function.Parameters != nil),
			zap.Strings("tool_"+strconv.Itoa(idx)+"_parameter_keys", paramKeys),
		)
	}
	lg.Debug("mcp tool schema snapshot", entries...)
}

// firstChoice returns the first chat choice when available.
func firstChoice(response *openai.TextResponse) (openai.TextResponseChoice, bool) {
	if response == nil || len(response.Choices) == 0 {
		return openai.TextResponseChoice{}, false
	}
	return response.Choices[0], true
}

// allMCPToolCalls checks if all tool calls are registered MCP tools.
func allMCPToolCalls(calls []relaymodel.Tool, registry *mcpToolRegistry) bool {
	if registry == nil {
		return false
	}
	for _, call := range calls {
		if call.Function == nil || call.Function.Name == "" {
			return false
		}
		if !registry.isMCPTool(call.Function.Name) {
			return false
		}
	}
	return true
}

// executeMCPToolCalls invokes MCP tools and returns tool result messages.
func executeMCPToolCalls(c *gin.Context, registry *mcpToolRegistry, calls []relaymodel.Tool, executed map[string]struct{}, summary *mcpExecutionSummary) ([]relaymodel.Message, error) {
	results := make([]relaymodel.Message, 0, len(calls))
	lg := gmw.GetLogger(c)
	for _, call := range calls {
		if call.Function == nil {
			continue
		}
		callID := strings.TrimSpace(call.Id)
		if callID != "" {
			if _, exists := executed[callID]; exists {
				continue
			}
			executed[callID] = struct{}{}
		}
		name := strings.TrimSpace(call.Function.Name)
		candidates := registry.candidatesByName[strings.ToLower(name)]
		if len(candidates) == 0 {
			continue
		}
		args, err := parseToolArguments(call.Function.Arguments)
		if err != nil {
			return nil, errors.Wrap(err, "parse tool arguments")
		}

		selected, result, err := mcp.CallWithFallback(gmw.Ctx(c), candidates, func(ctx context.Context, candidate mcp.ToolCandidate) (*mcp.CallToolResult, error) {
			server := resolveServerByID(candidate.ServerID)
			if server == nil {
				return nil, errors.New("mcp server not loaded")
			}
			headers := registry.requestHeaders[strings.ToLower(name)]
			client := mcp.NewStreamableHTTPClientWithLogger(server, headers, time.Duration(config.MCPToolCallTimeoutSec)*time.Second, lg)
			lg.Debug("invoking mcp tool",
				zap.String("tool", candidate.Tool.Name),
				zap.Int("server_id", candidate.ServerID),
				zap.String("server_label", candidate.ServerLabel),
			)
			return client.CallTool(ctx, candidate.Tool.Name, args)
		})
		if err != nil {
			return nil, err
		}
		msg, err := buildToolResultMessage(call.Id, result)
		if err != nil {
			return nil, err
		}
		results = append(results, msg)
		recordMCPToolUsage(summary, selected, name)
	}
	return results, nil
}

// parseToolArguments converts tool arguments into a JSON object.
func parseToolArguments(raw any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	switch v := raw.(type) {
	case map[string]any:
		return v, nil
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return map[string]any{}, nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	default:
		payload, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var parsed map[string]any
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	}
}

// buildToolResultMessage converts MCP tool output into a chat tool message.
func buildToolResultMessage(callID string, result *mcp.CallToolResult) (relaymodel.Message, error) {
	message := relaymodel.Message{Role: "tool", ToolCallId: callID}
	if result == nil {
		message.Content = ""
		return message, nil
	}
	if len(result.Raw) > 0 {
		message.Content = string(result.Raw)
		return message, nil
	}
	payload := map[string]any{"content": result.Content}
	if result.IsError {
		payload["is_error"] = true
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return relaymodel.Message{}, errors.Wrap(err, "marshal mcp tool result")
	}
	message.Content = string(encoded)
	return message, nil
}

// mergeUsage accumulates usage across tool rounds.
func mergeUsage(base *relaymodel.Usage, next *relaymodel.Usage) *relaymodel.Usage {
	if next == nil {
		return base
	}
	if base == nil {
		clone := *next
		return &clone
	}
	base.PromptTokens += next.PromptTokens
	base.CompletionTokens += next.CompletionTokens
	base.TotalTokens += next.TotalTokens
	base.ToolsCost += next.ToolsCost
	return base
}

// estimateMCPRoundPreConsumeQuota calculates the quota to pre-consume for a single MCP upstream round.
// Parameters: request is the chat request being sent, promptTokens is the estimated prompt token count, ratio is the pricing ratio.
// Returns: the quota amount that should be pre-consumed for the round.
func estimateMCPRoundPreConsumeQuota(request *relaymodel.GeneralOpenAIRequest, promptTokens int, ratio float64) int64 {
	if request == nil {
		return 0
	}
	preConsumedTokens := int64(promptTokens)
	if request.MaxCompletionTokens != nil && *request.MaxCompletionTokens > 0 {
		preConsumedTokens += int64(*request.MaxCompletionTokens)
	} else if request.MaxTokens > 0 {
		preConsumedTokens += int64(request.MaxTokens)
	}
	baseQuota := int64(float64(preConsumedTokens) * ratio)
	if ratio != 0 && baseQuota <= 0 && preConsumedTokens > 0 {
		baseQuota = 1
	}
	return baseQuota
}

// preConsumeMCPRoundQuota pre-consumes quota before each MCP upstream request.
// Parameters: c is the Gin context, meta provides request metadata, request is the upstream chat request, promptTokens is the estimated prompt tokens, ratio is the pricing ratio.
// Returns: the quota that was pre-consumed and any error encountered.
func preConsumeMCPRoundQuota(c *gin.Context, meta *metalib.Meta, request *relaymodel.GeneralOpenAIRequest, promptTokens int, ratio float64) (int64, error) {
	if c == nil || meta == nil {
		return 0, errors.New("context or meta is nil")
	}
	ctx := gmw.Ctx(c)
	preConsumedQuota := estimateMCPRoundPreConsumeQuota(request, promptTokens, ratio)
	if preConsumedQuota <= 0 {
		return 0, nil
	}
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return preConsumedQuota, err
	}
	if userQuota-preConsumedQuota < 0 {
		return preConsumedQuota, errors.New("user quota is not enough")
	}
	if err := model.CacheDecreaseUserQuota(ctx, meta.UserId, preConsumedQuota); err != nil {
		return preConsumedQuota, err
	}
	if err := model.PreConsumeTokenQuota(ctx, meta.TokenId, preConsumedQuota); err != nil {
		return preConsumedQuota, err
	}
	return preConsumedQuota, nil
}

// updateMCPRequestCostProvisional stores a provisional cost snapshot for the request.
// Parameters: c is the Gin context, meta provides request metadata, quota is the provisional quota value to store.
// Returns: none.
func updateMCPRequestCostProvisional(c *gin.Context, meta *metalib.Meta, quota int64) {
	if c == nil || meta == nil {
		return
	}
	requestId := c.GetString(ctxkey.RequestId)
	if requestId == "" {
		return
	}
	quotaId := c.GetInt(ctxkey.Id)
	if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
		gmw.GetLogger(c).Warn("record provisional mcp request cost failed", zap.Error(err), zap.String("request_id", requestId))
	}
}

// updateMCPRequestCostEstimate updates request cost based on accumulated usage after each round.
// Parameters: c is the Gin context, meta provides request metadata, usage is the accumulated usage, modelName/modelRatio/groupRatio/channelCompletionRatio/pricingAdaptor drive pricing.
// Returns: none.
func updateMCPRequestCostEstimate(c *gin.Context, meta *metalib.Meta, usage *relaymodel.Usage, modelName string, modelRatio float64, groupRatio float64, channelCompletionRatio map[string]float64, pricingAdaptor adaptor.Adaptor) {
	if c == nil || meta == nil || usage == nil {
		return
	}
	requestId := c.GetString(ctxkey.RequestId)
	if requestId == "" {
		return
	}
	quotaId := c.GetInt(ctxkey.Id)
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              modelName,
		ModelRatio:             modelRatio,
		GroupRatio:             groupRatio,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
	})
	quota := computeResult.TotalQuota + usage.ToolsCost
	if quota < 0 {
		quota = 0
	}
	if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
		gmw.GetLogger(c).Warn("record mcp request cost estimate failed", zap.Error(err), zap.String("request_id", requestId))
	}
}

// resolveMCPToolCostSnapshot returns the current total MCP tool cost for delta calculations.
func resolveMCPToolCostSnapshot(summary *mcpExecutionSummary) int64 {
	if summary == nil || summary.summary == nil {
		return 0
	}
	return summary.summary.TotalCost
}

// applyMCPToolCostDelta applies incremental MCP tool costs to usage for billing.
func applyMCPToolCostDelta(usage *relaymodel.Usage, previousCost int64, summary *mcpExecutionSummary) *relaymodel.Usage {
	if summary == nil || summary.summary == nil {
		return usage
	}
	costDelta := summary.summary.TotalCost - previousCost
	if costDelta <= 0 {
		return usage
	}
	if usage == nil {
		return &relaymodel.Usage{ToolsCost: costDelta}
	}
	usage.ToolsCost += costDelta
	if usage.TotalTokens == 0 && (usage.PromptTokens != 0 || usage.CompletionTokens != 0) {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

// recordMCPToolUsage updates tool usage summary for MCP tool calls.
func recordMCPToolUsage(summary *mcpExecutionSummary, selected mcp.ToolCandidate, name string) {
	if summary == nil || summary.summary == nil {
		return
	}
	qualifiedName := name
	if selected.ServerLabel != "" {
		qualifiedName = selected.ServerLabel + "." + name
	}
	cost := resolveMCPToolCost(selected.Policy.Pricing)
	summary.summary.TotalCost += cost
	summary.summary.Counts[qualifiedName]++
	if cost > 0 {
		summary.summary.CostByTool[qualifiedName] += cost
	}
	summary.summary.Entries = append(summary.summary.Entries, model.ToolUsageEntry{
		Tool:     qualifiedName,
		Source:   mcpToolSourceOneAPI,
		ServerID: selected.ServerID,
		Count:    1,
		Cost:     cost,
	})
}

// resolveMCPToolCost converts MCP pricing to quota units.
func resolveMCPToolCost(pricing model.ToolPricingLocal) int64 {
	if pricing.QuotaPerCall > 0 {
		return pricing.QuotaPerCall
	}
	if pricing.UsdPerCall > 0 {
		return int64(float64(ratio.QuotaPerUsd) * pricing.UsdPerCall)
	}
	return 0
}

// resolveServerByID loads an MCP server by ID.
func resolveServerByID(id int) *model.MCPServer {
	if id <= 0 {
		return nil
	}
	server, err := model.GetMCPServerByID(id)
	if err != nil {
		return nil
	}
	return server
}

// getRelayUserFromContext loads the authenticated user for relay controllers.
func getRelayUserFromContext(c *gin.Context) (*model.User, error) {
	if c == nil {
		return nil, errors.New("context is nil")
	}
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

// splitMCPToolName splits server-qualified tool names into label and tool name.
func splitMCPToolName(name string) (string, string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

// mergeToolUsageSummaries merges MCP summary into existing summary when available.
func mergeToolUsageSummaries(existing *model.ToolUsageSummary, addition *model.ToolUsageSummary) *model.ToolUsageSummary {
	if existing == nil {
		return addition
	}
	if addition == nil {
		return existing
	}
	if existing.Counts == nil {
		existing.Counts = map[string]int{}
	}
	if existing.CostByTool == nil {
		existing.CostByTool = map[string]int64{}
	}
	for name, count := range addition.Counts {
		existing.Counts[name] += count
	}
	for name, cost := range addition.CostByTool {
		existing.CostByTool[name] += cost
	}
	existing.TotalCost += addition.TotalCost
	existing.Entries = append(existing.Entries, addition.Entries...)
	return existing
}

// normalizeMCPToolChoiceForResponse coerces response tool_choice to function for MCP aliases.
func normalizeMCPToolChoiceForResponse(choice any, mcpNames map[string]struct{}) any {
	if choice == nil || len(mcpNames) == 0 {
		return choice
	}
	mapChoice, ok := choice.(map[string]any)
	if !ok {
		return choice
	}
	typeVal, _ := mapChoice["type"].(string)
	name, _ := mapChoice["name"].(string)
	if name == "" {
		name = strings.TrimSpace(typeVal)
	}
	if name == "" {
		return choice
	}
	if _, exists := mcpNames[strings.ToLower(name)]; !exists {
		return choice
	}
	return map[string]any{
		"type": "function",
		"name": name,
	}
}
