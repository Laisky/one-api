package quota

import (
	"math"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/pricing"
)

// computeGenerationQuota prices exactly one already-admitted media generation.
// Parameters: input carries the admission tariff; resolved is the already-read
// token pricing configuration used to avoid another lookup on unrelated models.
// Returns: the flat quota and true when applicable, retaining an explicit billing
// issue for an invalid tariff so callers cannot mistake unknown pricing for free.
func computeGenerationQuota(input ComputeInput, resolved adaptor.ModelConfig) (ComputeResult, bool) {
	// Preserve the token path's single pricing lookup. Only explicit flat-rate
	// candidates need the full generation contract and its media metadata.
	candidate := resolved.PricingProvenance != nil && resolved.PricingProvenance.Unit == "generation"
	if local, ok := input.ChannelModelConfigs[input.ModelName]; ok {
		candidate = candidate || local.PerCall != nil
		for _, window := range local.TimeWindows {
			candidate = candidate || window.Overlay.PerCall != nil
		}
	}
	if !candidate {
		return ComputeResult{}, false
	}
	tariff, applies, err := pricing.ResolveGenerationTariff(input.ModelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime)
	if !applies {
		return ComputeResult{}, false
	}
	result := ComputeResult{PromptTokens: input.Usage.PromptTokens, CompletionTokens: input.Usage.CompletionTokens}
	if err != nil {
		result.UnpricedUsage = true
		result.BillingIssues = []string{"generation tariff could not be resolved"}
		return result, true
	}
	amount, err := pricing.GenerationQuota(tariff, input.GroupRatio)
	if err != nil || input.Usage.ToolsCost < 0 || input.Usage.ToolsCost > math.MaxInt64-amount {
		result.UnpricedUsage = true
		result.BillingIssues = []string{"generation quota is invalid or exceeds supported range"}
		return result, true
	}
	result.TotalQuota = amount + input.Usage.ToolsCost
	return result, true
}
