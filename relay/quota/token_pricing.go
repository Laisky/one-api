package quota

import (
	"math"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/pricing"
)

// tokenPricing holds the effective tariff and scalar precedence shared by admission and settlement.
type tokenPricing struct {
	effective         pricing.EffectivePricing
	input, completion float64
}

// resolveTokenPricing selects usage tiers while retaining channel scalar overrides
// and fallback ratios. Parameters: input carries usage and pricing context, cfg is
// the resolved request-time configuration and known reports its presence.
// Returns: the normal and cache rates used by both quotes and durable settlement.
func resolveTokenPricing(input ComputeInput, resolvedModelCfg adaptor.ModelConfig, hasResolvedModelCfg bool) tokenPricing {
	pricingAdaptor := input.PricingAdaptor
	hasChannelModelRatioOverride := hasModelRatioFlatOverride(input.ModelName, input.ChannelModelRatio, input.ChannelModelConfigs)
	baseRatio := input.ModelRatio
	completionRatioResolved := resolveCompletionRatio(input.ModelName, resolvedModelCfg, hasResolvedModelCfg, input.ChannelCompletionRatio, input.ChannelModelConfigs, pricingAdaptor, input.RequestTime)

	if hasResolvedModelCfg {
		// Preserve legacy fallback behavior: when channel config omits base ratio/completion
		// (keeps zero values), continue using the resolved three-layer ratios as base values.
		if resolvedModelCfg.Ratio == 0 {
			resolvedModelCfg.Ratio = baseRatio
		}
		if resolvedModelCfg.CompletionRatio == 0 {
			resolvedModelCfg.CompletionRatio = completionRatioResolved
		}
	} else {
		// Build a minimal config from resolved base ratios if no config was found.
		resolvedModelCfg = adaptor.ModelConfig{
			Ratio:           baseRatio,
			CompletionRatio: completionRatioResolved,
		}
	}

	tierPromptTokens := promptTokensForTier(input.ModelName, input.Usage)
	eff := pricing.ResolveEffectivePricingForUsageFromConfig(tierPromptTokens, input.Usage.CompletionTokens, resolvedModelCfg)

	usedModelRatio := baseRatio
	usedCompletionRatio := completionRatioResolved

	if hasResolvedModelCfg {
		if !hasChannelModelRatioOverride {
			usedModelRatio = eff.InputRatio
		}
		baseComp := eff.OutputRatio
		completionBaseRatio := eff.InputRatio
		if hasChannelModelRatioOverride {
			completionBaseRatio = usedModelRatio
			baseComp = usedModelRatio * completionRatioResolved
			for _, tier := range resolvedModelCfg.Tiers {
				if !pricing.TierApplies(tierPromptTokens, input.Usage.CompletionTokens, tier) {
					continue
				}
				if tier.CompletionRatio != 0 {
					baseComp = usedModelRatio * tier.CompletionRatio
				}
			}
		}
		if completionBaseRatio != 0 {
			baseComp = baseComp / completionBaseRatio
		} else {
			baseComp = 1.0
		}
		usedCompletionRatio = baseComp
	} else if pricingAdaptor != nil {
		// Optimized check: only use effective pricing if the input model ratio matches the adaptor base.
		// This avoids extra GetDefaultModelPricing() map lookups when not needed.
		adaptorBase := pricingAdaptor.GetModelRatio(input.ModelName)
		if math.Abs(baseRatio-adaptorBase) < 1e-12 {
			usedModelRatio = eff.InputRatio
			baseComp := eff.OutputRatio
			if eff.InputRatio != 0 {
				baseComp = eff.OutputRatio / eff.InputRatio
			} else {
				baseComp = 1.0
			}
			usedCompletionRatio = baseComp
		}
	}

	return tokenPricing{effective: eff, input: usedModelRatio, completion: usedCompletionRatio}
}
