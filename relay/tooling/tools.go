package tooling

import (
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// ValidateChatBuiltinTools ensures the chat request only references built-in tools allowed for the channel/model.
func ValidateChatBuiltinTools(c *gin.Context, request *relaymodel.GeneralOpenAIRequest, meta *metalib.Meta, channel *model.Channel, provider adaptor.Adaptor) error {
	if request == nil {
		return nil
	}

	requested := CollectChatBuiltins(request)
	if len(requested) == 0 {
		return nil
	}

	modelName := resolveModelName(meta, request.Model)
	return ValidateRequestedBuiltins(modelName, meta, channel, provider, requested)
}

// PruneDisallowedResponseBuiltins removes built-in tools from the Response API request when the
// channel policy explicitly disallows them and the client did not force a specific tool choice.
// Returns the list of canonical tool names that were removed.
func PruneDisallowedResponseBuiltins(request *openai.ResponseAPIRequest, meta *metalib.Meta, channel *model.Channel, provider adaptor.Adaptor) []string {
	if request == nil || !canPruneResponseToolChoice(request.ToolChoice) {
		return nil
	}

	modelName := resolveModelName(meta, request.Model)
	effectiveProvider := provider
	if channel != nil {
		switch channel.Type {
		case channeltype.Azure:
			effectiveProvider = nil
		}
	} else if meta != nil {
		switch meta.ChannelType {
		case channeltype.Azure:
			effectiveProvider = nil
		}
	}

	policy := buildToolPolicy(channel, effectiveProvider, modelName)
	filtered := make([]openai.ResponseAPITool, 0, len(request.Tools))
	removedSet := make(map[string]struct{})

	for _, tool := range request.Tools {
		name := NormalizeBuiltinType(tool.Type)
		if name == "" || policy.isAllowed(name) {
			filtered = append(filtered, tool)
			continue
		}
		removedSet[name] = struct{}{}
	}

	if len(removedSet) == 0 {
		return nil
	}

	request.Tools = filtered
	removed := make([]string, 0, len(removedSet))
	for name := range removedSet {
		removed = append(removed, name)
	}
	return removed
}

func canPruneResponseToolChoice(choice any) bool {
	if choice == nil {
		return true
	}
	if s, ok := choice.(string); ok {
		trimmed := strings.ToLower(strings.TrimSpace(s))
		return trimmed == "" || trimmed == "auto"
	}
	return false
}

// ApplyBuiltinToolCharges reconciles built-in tool usage with billing by updating usage.ToolsCost and
// recording metadata for downstream logging.
func ApplyBuiltinToolCharges(c *gin.Context, usage **relaymodel.Usage, meta *metalib.Meta, channel *model.Channel, provider adaptor.Adaptor) {
	if c == nil || meta == nil {
		return
	}

	counts := collectInvocationCounts(c)
	if len(counts) == 0 {
		return
	}

	modelName := resolveModelName(meta, meta.ActualModelName)
	policy := buildToolPolicy(channel, provider, modelName)

	ensureUsage := func() *relaymodel.Usage {
		if usage == nil {
			return nil
		}
		if *usage == nil {
			*usage = &relaymodel.Usage{}
		}
		return *usage
	}

	summary := &model.ToolUsageSummary{
		Counts:     make(map[string]int),
		CostByTool: make(map[string]int64),
	}

	var totalCost int64
	logger := gmw.GetLogger(c)

	for rawName, count := range counts {
		if count <= 0 {
			continue
		}
		canonical := strings.ToLower(rawName)
		summary.Counts[canonical] = count

		if !policy.isAllowed(canonical) {
			logger.Warn("builtin tool invocation ignored due to policy", zap.String("tool", canonical), zap.String("model", modelName))
			continue
		}

		perCall := policy.pricing[canonical]
		cost := perCall * int64(count)
		if usagePtr := ensureUsage(); usagePtr != nil {
			usagePtr.ToolsCost += cost
			// Maintain TotalTokens when upstream omits it to align with previous behaviour.
			if usagePtr.TotalTokens == 0 && (usagePtr.PromptTokens != 0 || usagePtr.CompletionTokens != 0) {
				usagePtr.TotalTokens = usagePtr.PromptTokens + usagePtr.CompletionTokens
			}
		}
		if cost != 0 {
			logger.Debug("applied builtin tool cost", zap.String("tool", canonical), zap.Int("count", count), zap.Int64("quota_cost", cost))
			summary.CostByTool[canonical] = cost
			totalCost += cost
		}
	}

	summary.TotalCost = totalCost
	if len(summary.Counts) == 0 && totalCost == 0 {
		return
	}

	c.Set(ctxkey.ToolInvocationSummary, summary)
}

// collectInvocationCounts normalizes tool invocation counters stored on the gin context.
func collectInvocationCounts(c *gin.Context) map[string]int {
	counts := make(map[string]int)

	if raw, ok := c.Get(ctxkey.ToolInvocationCounts); ok {
		mergeToolCounts(counts, raw)
	}

	if raw, ok := c.Get(ctxkey.WebSearchCallCount); ok {
		counts["web_search"] += toInt(raw)
	}

	// Remove zero or negative entries to keep downstream handling simple.
	for name, count := range counts {
		if count <= 0 {
			delete(counts, name)
		}
	}

	return counts
}

// mergeToolCounts merges arbitrary typed maps containing tool invocation counters into the accumulator map.
func mergeToolCounts(dst map[string]int, src any) {
	switch typed := src.(type) {
	case map[string]int:
		for name, count := range typed {
			dst[strings.ToLower(name)] += count
		}
	case map[string]int64:
		for name, count := range typed {
			dst[strings.ToLower(name)] += int(count)
		}
	case map[string]float64:
		for name, count := range typed {
			dst[strings.ToLower(name)] += int(count)
		}
	case map[string]any:
		for name, value := range typed {
			dst[strings.ToLower(name)] += toInt(value)
		}
	}
}

// toInt converts different numeric representations to int, defaulting to zero on failure.
func toInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

// CollectChatBuiltins extracts built-in tool identifiers from a chat completion request.
func CollectChatBuiltins(request *relaymodel.GeneralOpenAIRequest) map[string]struct{} {
	builtins := make(map[string]struct{})

	for _, tool := range request.Tools {
		if name := NormalizeBuiltinType(tool.Type); name != "" {
			builtins[name] = struct{}{}
		}
	}

	if request.WebSearchOptions != nil {
		builtins["web_search"] = struct{}{}
	}

	return builtins
}

// CollectResponseBuiltins extracts built-in tool identifiers from a Response API request.
func CollectResponseBuiltins(request *openai.ResponseAPIRequest) map[string]struct{} {
	builtins := make(map[string]struct{})
	if request == nil {
		return builtins
	}
	for _, tool := range request.Tools {
		if name := NormalizeBuiltinType(tool.Type); name != "" {
			builtins[name] = struct{}{}
		}
	}
	return builtins
}

// NormalizeBuiltinType normalizes known built-in tool identifiers (case-insensitive). Unknown tools return "".
func NormalizeBuiltinType(toolType string) string {
	switch strings.ToLower(strings.TrimSpace(toolType)) {
	case "web_search":
		return "web_search"
	case "web_search_preview":
		return "web_search"
	default:
		return ""
	}
}

// ValidateResponseBuiltinTools verifies built-in tool usage on Response API requests prior to fallback conversions.
func ValidateResponseBuiltinTools(request *openai.ResponseAPIRequest, meta *metalib.Meta, channel *model.Channel, provider adaptor.Adaptor) error {
	if request == nil {
		return nil
	}
	requested := CollectResponseBuiltins(request)
	if len(requested) == 0 {
		return nil
	}
	modelName := resolveModelName(meta, request.Model)
	return ValidateRequestedBuiltins(modelName, meta, channel, provider, requested)
}

// ValidateRequestedBuiltins verifies the requested built-in tools against channel/provider policy.
func ValidateRequestedBuiltins(modelName string, meta *metalib.Meta, channel *model.Channel, provider adaptor.Adaptor, requested map[string]struct{}) error {
	if len(requested) == 0 {
		return nil
	}

	effectiveModel := resolveModelName(meta, modelName)
	effectiveProvider := provider
	if channel != nil {
		switch channel.Type {
		case channeltype.Azure:
			effectiveProvider = nil
		}
	} else if meta != nil {
		switch meta.ChannelType {
		case channeltype.Azure:
			effectiveProvider = nil
		}
	}
	policy := buildToolPolicy(channel, effectiveProvider, effectiveModel)
	for toolName := range requested {
		if !policy.isAllowed(toolName) {
			return errors.Errorf("tool %s is not allowed on this channel (model=%s); update the tooling whitelist or pricing", toolName, effectiveModel)
		}
	}

	return nil
}

// toolPolicy models the effective allowlist and pricing for built-in tools.
type toolPolicy struct {
	whitelistDefined bool
	allowed          map[string]struct{}
	pricing          map[string]int64
	channelPricing   map[string]struct{}
	providerPricing  map[string]struct{}
}

func (p toolPolicy) hasPricing(tool string) bool {
	canonical := strings.ToLower(strings.TrimSpace(tool))
	if canonical == "" {
		return false
	}
	if _, ok := p.channelPricing[canonical]; ok {
		return true
	}
	if _, ok := p.providerPricing[canonical]; ok {
		return true
	}
	return false
}

func (p toolPolicy) hasChannelPricing(tool string) bool {
	_, ok := p.channelPricing[strings.ToLower(strings.TrimSpace(tool))]
	return ok
}

func (p toolPolicy) hasProviderPricing(tool string) bool {
	_, ok := p.providerPricing[strings.ToLower(strings.TrimSpace(tool))]
	return ok
}

// isAllowed reports whether the tool is permitted under the effective policy.
func (p toolPolicy) isAllowed(tool string) bool {
	canonical := strings.ToLower(strings.TrimSpace(tool))
	if canonical == "" {
		return false
	}
	if !p.hasPricing(canonical) {
		return false
	}
	if p.whitelistDefined {
		_, ok := p.allowed[canonical]
		return ok
	}
	if _, ok := p.allowed[canonical]; ok {
		return true
	}
	return true
}

// buildToolPolicy merges channel overrides with provider defaults to construct the effective policy.
func buildToolPolicy(channel *model.Channel, provider adaptor.Adaptor, _ string) toolPolicy {
	policy := toolPolicy{
		allowed:         make(map[string]struct{}),
		pricing:         make(map[string]int64),
		channelPricing:  make(map[string]struct{}),
		providerPricing: make(map[string]struct{}),
	}

	setWhitelist := func(list []string) {
		if len(list) == 0 {
			return
		}
		policy.whitelistDefined = true
		policy.allowed = make(map[string]struct{}, len(list))
		for _, name := range list {
			canonical := strings.ToLower(strings.TrimSpace(name))
			if canonical == "" {
				continue
			}
			policy.allowed[canonical] = struct{}{}
		}
	}

	applyProviderPricing := func(pricing map[string]adaptor.ToolPricingConfig) {
		for name, cfg := range pricing {
			canonical := strings.ToLower(strings.TrimSpace(name))
			if canonical == "" {
				continue
			}
			policy.pricing[canonical] = quotaPerCallFromProvider(cfg)
			policy.providerPricing[canonical] = struct{}{}
		}
	}

	applyChannelPricing := func(pricing map[string]model.ToolPricingLocal) {
		for name, cfg := range pricing {
			canonical := strings.ToLower(strings.TrimSpace(name))
			if canonical == "" {
				continue
			}
			policy.pricing[canonical] = quotaPerCallFromLocal(cfg)
			policy.channelPricing[canonical] = struct{}{}
		}
	}

	var providerTooling *adaptor.ChannelToolConfig
	if provider != nil {
		if defaults, ok := provider.(adaptor.ToolingDefaultsProvider); ok {
			cfg := defaults.DefaultToolingConfig()
			providerTooling = &cfg
		}
	}

	if providerTooling != nil {
		if len(providerTooling.Whitelist) > 0 {
			setWhitelist(providerTooling.Whitelist)
		}
		applyProviderPricing(providerTooling.Pricing)
	}

	var channelTooling *model.ChannelToolingConfig
	if channel != nil {
		channelTooling = channel.GetToolingConfig()
	}

	if channelTooling != nil {
		if len(channelTooling.Whitelist) > 0 {
			setWhitelist(channelTooling.Whitelist)
		}
		applyChannelPricing(channelTooling.Pricing)
	}

	return policy
}

// quotaPerCallFromLocal converts channel-local pricing definition to quota units.
func quotaPerCallFromLocal(pricing model.ToolPricingLocal) int64 {
	if pricing.QuotaPerCall > 0 {
		return pricing.QuotaPerCall
	}
	if pricing.UsdPerCall > 0 {
		return int64(math.Ceil(pricing.UsdPerCall * float64(ratio.QuotaPerUsd)))
	}
	return pricing.QuotaPerCall
}

// quotaPerCallFromProvider converts provider default pricing to quota units.
func quotaPerCallFromProvider(pricing adaptor.ToolPricingConfig) int64 {
	if pricing.QuotaPerCall > 0 {
		return pricing.QuotaPerCall
	}
	if pricing.UsdPerCall > 0 {
		return int64(math.Ceil(pricing.UsdPerCall * float64(ratio.QuotaPerUsd)))
	}
	return pricing.QuotaPerCall
}

// resolveModelName returns the most appropriate model identifier for policy resolution.
func resolveModelName(meta *metalib.Meta, fallback string) string {
	if meta != nil {
		if strings.TrimSpace(meta.ActualModelName) != "" {
			return meta.ActualModelName
		}
		if strings.TrimSpace(meta.OriginModelName) != "" {
			return meta.OriginModelName
		}
	}
	return strings.TrimSpace(fallback)
}
