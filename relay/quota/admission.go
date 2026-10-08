package quota

import (
	"math"

	"github.com/Laisky/errors/v2"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// AdmissionOptions names the cache creation buckets permitted by the canonical request.
type AdmissionOptions struct {
	CacheWrite5m, CacheWrite1h bool
	OutputCount                int
}

// EstimateTierAdmission prices the full estimated input once and every possible
// output tier through maxOutputTokens before paid dispatch. Parameters: input
// carries canonical prompt usage and request-time pricing; bufferTokens is the
// existing extra normal-input allowance. Returns: the conservative rounded quote,
// whether a resolved tier contract applies, and an error for invalid arithmetic.
// Unknown cache composition uses the highest input bucket rate without discounts.
func EstimateTierAdmission(input ComputeInput, maxOutputTokens int, bufferTokens int64, options AdmissionOptions) (int64, bool, error) {
	cfg, known := pricing.ResolveModelConfigRatioOnly(input.ModelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime)
	if !known || len(cfg.Tiers) == 0 {
		return 0, false, nil
	}
	if input.Usage == nil || input.Usage.PromptTokens < 0 || maxOutputTokens < 0 || bufferTokens < 0 {
		return 0, true, errors.New("invalid tier admission token estimate")
	}
	if maxOutputTokens == 0 {
		full, _ := pricing.ResolveModelConfig(input.ModelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime)
		maxOutputTokens = int(full.MaxOutputTokens)
		if maxOutputTokens <= 0 {
			maxOutputTokens = int(full.MaxTokens)
		}
		if maxOutputTokens <= 0 {
			provider, _ := pricing.ResolveModelConfig(input.ModelName, nil, input.PricingAdaptor, input.RequestTime)
			maxOutputTokens = int(provider.MaxOutputTokens)
		}
		if maxOutputTokens <= 0 {
			return 0, true, errors.New("tiered request has no bounded output limit")
		}
	}
	count := options.OutputCount
	if count == 0 {
		count = 1
	}
	if count < 1 || maxOutputTokens > math.MaxInt/count {
		return 0, true, errors.New("tier admission output count exceeds integer range")
	}
	maxOutputTokens *= count
	options.CacheWrite5m = options.CacheWrite5m || input.Usage.CacheWrite5mTokens > 0
	options.CacheWrite1h = options.CacheWrite1h || input.Usage.CacheWrite1hTokens > 0
	prompt := promptTokensForTier(input.ModelName, input.Usage)
	counts := []int{0, maxOutputTokens}
	for _, tier := range cfg.Tiers {
		if tier.OutputTokenThreshold > 0 && tier.OutputTokenThreshold <= maxOutputTokens {
			counts = append(counts, tier.OutputTokenThreshold, tier.OutputTokenThreshold-1)
		}
	}
	highest := 0.0
	for _, output := range counts {
		candidate := input
		candidate.Usage = &relaymodel.Usage{PromptTokens: prompt, CompletionTokens: output}
		rates := resolveTokenPricing(candidate, cfg, known)
		normal := rates.input * input.GroupRatio
		completion := normal * rates.completion
		inputRate := normal
		cacheRates := []float64{rates.effective.CachedInputRatio}
		if options.CacheWrite5m {
			cacheRates = append(cacheRates, rates.effective.CacheWrite5mRatio)
		}
		if options.CacheWrite1h {
			cacheRates = append(cacheRates, rates.effective.CacheWrite1hRatio)
		}
		for _, rate := range cacheRates {
			if rate == 0 {
				rate = rates.input
			} else if rate < 0 {
				rate = 0
			}
			inputRate = max(inputRate, rate*input.GroupRatio)
		}
		for _, rate := range []float64{normal, completion, inputRate} {
			if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
				return 0, true, errors.New("invalid tier admission ratio")
			}
		}
		cost := float64(prompt)*inputRate + float64(output)*completion + float64(bufferTokens)*normal
		if math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 || math.Ceil(cost) >= float64(math.MaxInt64) {
			return 0, true, errors.New("tier admission quote exceeds integer range")
		}
		highest = max(highest, cost)
	}
	return int64(math.Ceil(highest)), true, nil
}
