package controller

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/mcp"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

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
