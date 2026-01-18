package controller

import (
	"encoding/json"
	"strconv"
	"strings"

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
	"github.com/songquanpeng/one-api/relay/mcp"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/tooling"
)

const (
	mcpToolSourceOneAPI = "oneapi_builtin"
)

type mcpToolRegistry struct {
	candidatesByName map[string][]mcp.ToolCandidate
	requestHeaders   map[string]map[string]string
	originalTools    []relaymodel.Tool
	toolNameByType   map[string]string
	selectedIndex    map[string]int
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
		originalTools:    append([]relaymodel.Tool(nil), request.Tools...),
		toolNameByType:   make(map[string]string),
		selectedIndex:    make(map[string]int),
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
				normalizedType := strings.ToLower(strings.TrimSpace(tool.Type))
				if normalizedType != "" {
					registry.toolNameByType[normalizedType] = name
				}
				registry.selectedIndex[name] = 0
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
		maxRounds = 10
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

		messageStart := len(request.Messages)
		request.Messages = append(request.Messages, choice.Message)
		previousCost := resolveMCPToolCostSnapshot(summary)
		results, execErr := executeMCPToolCalls(c, registry, choice.Message.ToolCalls, executedToolCalls, summary)
		if execErr != nil {
			var schemaErr *mcpToolSchemaMismatchError
			if errors.As(execErr, &schemaErr) {
				if registry.setSelectedCandidate(schemaErr.ToolName, schemaErr.CandidateIndex) {
					if rebuildErr := registry.rebuildRequestTools(request); rebuildErr != nil {
						return nil, accumulated, summary, incrementalCharged, openai.ErrorWrapper(rebuildErr, "mcp_tool_rebuild_failed", 500)
					}
					request.Messages = request.Messages[:messageStart]
					for _, callID := range schemaErr.CallIDs {
						delete(executedToolCalls, callID)
					}
					lg.Debug("mcp tool schema mismatch triggers upstream retry",
						zap.String("tool", schemaErr.ToolName),
						zap.Int("candidate_index", schemaErr.CandidateIndex),
					)
					continue
				}
			}
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
		nameKey := strings.ToLower(name)
		candidates := registry.candidatesByName[nameKey]
		if len(candidates) == 0 {
			continue
		}
		args, err := parseToolArguments(call.Function.Arguments)
		if err != nil {
			return nil, errors.Wrap(err, "parse tool arguments")
		}

		startIndex := registry.selectedCandidateIndex(nameKey)
		selected, result, err := callMCPToolWithFallback(c, registry, nameKey, args, candidates, startIndex, []string{callID})
		if err != nil {
			return nil, err
		}
		msg, err := buildToolResultMessage(call.Id, result)
		if err != nil {
			return nil, err
		}
		results = append(results, msg)
		recordMCPToolUsage(summary, selected, name)
		lg.Debug("mcp tool call completed",
			zap.String("tool", name),
			zap.Int("server_id", selected.ServerID),
			zap.String("server_label", selected.ServerLabel),
		)
	}
	return results, nil
}
