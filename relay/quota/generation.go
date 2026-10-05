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
	// Preserve the token path's single pricing lookup. Only catalog generation
	// contracts, or channel overrides that hide the catalog provenance (including
	// metadata-only "Load Default" snapshots), need the full generation resolver.
	candidate := resolved.PricingProvenance != nil && resolved.PricingProvenance.Unit == adaptor.TariffUnitGeneration
	if _, ok := input.ChannelModelConfigs[input.ModelName]; ok {
		candidate = true
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
