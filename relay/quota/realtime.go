package quota

import (
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/realtime"
)

// ErrRealtimePriceUnavailable identifies receipts with one or more unpriceable
// buckets. Resolved buckets remain chargeable; the result is a lower bound.
var ErrRealtimePriceUnavailable = errors.New("realtime price unavailable")

// computeRealtime prices each authoritative receipt under its own model and
// context tier, then rounds the session once. Unpriceable buckets stay explicitly
// unresolved but cannot erase the priceable portions of the same receipt.
func computeRealtime(input ComputeInput) ComputeResult {
	ledger := input.Usage.Realtime
	result := ComputeResult{PromptTokens: int(ledger.InputTokens), CompletionTokens: int(ledger.OutputTokens),
		UsedModelRatio: input.ModelRatio}
	for _, issue := range ledger.Issues {
		appendRealtimeBillingIssue(&result, issue)
	}
	var total, correction float64
	for _, record := range ledger.Records {
		rates, ratios, rateErr := resolveRealtimeRecordRates(input, record)
		if record.Model == "" {
			result.UsedModelRatio, result.UsedCompletionRatio = ratios.UsedModelRatio, ratios.UsedCompletionRatio
		}
		if rateErr != nil {
			result.UnpricedUsage = true
			appendRealtimeBillingIssue(&result, rateErr.Error())
		}
		t := record.Tokens
		result.CachedPromptTokens += int(t.CachedText + t.CachedAudio + t.CachedImage + t.CachedUnallocated)
		if t.CachedUnallocated > 0 && len(ledger.Issues) == 0 {
			appendRealtimeBillingIssue(&result, realtime.ErrAmbiguousCache.Error())
		}
		// Missing rates are zero only for the unresolved buckets. The issue
		// above prevents that lower bound from masquerading as exact billing.
		cost, err := realtime.Cost(record, rates)
		if err != nil {
			result.UnpricedUsage = true
			appendRealtimeBillingIssue(&result, err.Error())
			continue
		}
		// Compensated summation avoids per-turn floating-point accumulation drift.
		y := cost - correction
		next := total + y
		correction = (next - total) - y
		total = next
	}
	quota, err := realtime.RoundQuota(total, input.GroupRatio, input.Usage.ToolsCost)
	if err != nil {
		result.UnpricedUsage = true
		appendRealtimeBillingIssue(&result, err.Error())
		return result
	}
	result.TotalQuota = quota
	return result
}

// appendRealtimeBillingIssue bounds both the count and bytes of result's log
// diagnostics, including errors containing an administrator-provided model name.
func appendRealtimeBillingIssue(result *ComputeResult, message string) {
	if len(result.BillingIssues) >= realtime.MaxIssues {
		return
	}
	if len(message) > 256 {
		message = message[:256]
	}
	result.BillingIssues = append(result.BillingIssues, message)
}

// resolveRealtimeRecordRates resolves channel, provider/global, context-tier and
// time-window prices for one receipt. On partial failure it returns every known
// rate, zero for unknown buckets, and a classifiable error. Cache supplements
// remain relative to configured modality prices, preserving existing markups.
func resolveRealtimeRecordRates(input ComputeInput, record realtime.Record) (realtime.Rates, ComputeResult, error) {
	modelName := input.ModelName
	if record.Model != "" {
		modelName = record.Model
	}
	cfg, known := pricing.ResolveModelConfigRatioOnly(modelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime)
	if !known {
		return realtime.Rates{}, ComputeResult{}, errors.Wrapf(ErrRealtimePriceUnavailable, "model %q", modelName)
	}
	probe := input
	probe.ModelName = modelName
	probe.GroupRatio = 1
	probe.Usage = &relaymodel.Usage{PromptTokens: int(record.Tokens.Input), CompletionTokens: int(record.Tokens.Output)}
	if record.Model != "" {
		probe.ModelRatio = pricing.ResolveModelRatioAt(modelName, input.ChannelModelConfigs, input.ChannelModelRatio, input.PricingAdaptor, input.RequestTime)
	}
	resolved := Compute(probe)
	rates := realtime.Rates{Text: resolved.UsedModelRatio, OutputText: resolved.UsedModelRatio * resolved.UsedCompletionRatio}
	effective := pricing.ResolveEffectivePricingForUsageFromConfig(probe.Usage.PromptTokens, probe.Usage.CompletionTokens, cfg)
	rates.CachedText = rates.Text
	if effective.CachedInputRatio < 0 {
		rates.CachedText = 0
	} else if effective.CachedInputRatio > 0 {
		rates.CachedText = effective.CachedInputRatio
	}
	audio, hasAudio := pricing.ResolveAudioPricing(modelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime)
	if record.Duration {
		if !hasAudio || audio.UsdPerSecond <= 0 {
			return rates, resolved, errors.Wrapf(ErrRealtimePriceUnavailable, "duration for model %q", modelName)
		}
		rates.Second = audio.UsdPerSecond * billingratio.QuotaPerUsd
		return rates, resolved, nil
	}
	if rates.Text == 0 {
		// Explicit free token pricing must not gain a synthetic reservation fee.
		return realtime.Rates{}, resolved, nil
	}
	if hasAudio && audio.UsdPerSecond > 0 {
		return realtime.Rates{}, resolved, errors.Wrap(realtime.ErrInvalidUsage, "token receipt for duration-priced model")
	}
	var missing []string
	if hasAudio {
		promptRatio, completionRatio := audio.PromptRatio, audio.CompletionRatio
		if promptRatio == 0 {
			promptRatio = pricing.DefaultAudioPromptRatio
		}
		if completionRatio == 0 {
			completionRatio = pricing.DefaultAudioCompletionRatio
		}
		rates.Audio = rates.Text * promptRatio
		rates.OutputAudio = rates.Audio * completionRatio
	} else if record.Tokens.Audio > 0 || record.Tokens.OutputAudio > 0 {
		missing = append(missing, "audio")
	}
	cachedAudioDiscount, imageMultiplier, hasSupplement := realtimeSupplement(modelName)
	if hasSupplement {
		rates.CachedAudio = rates.Audio * cachedAudioDiscount
		rates.Image = rates.Text * imageMultiplier
		rates.CachedImage = rates.Image * 0.1
	}
	hasImageCache := hasSupplement && imageMultiplier > 0
	if image, ok := pricing.ResolveImagePricing(modelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime); ok && image.PromptRatio > 0 {
		rates.Image = rates.Text * image.PromptRatio
		rates.CachedImage = rates.Image * 0.1
		hasImageCache = true
	}
	needsAudioCache := record.Tokens.CachedAudio > 0 || (record.Tokens.CachedUnallocated > 0 && record.Tokens.Audio > 0)
	needsImageCache := record.Tokens.CachedImage > 0 || (record.Tokens.CachedUnallocated > 0 && record.Tokens.Image > 0)
	if needsAudioCache && !hasSupplement {
		missing = append(missing, "audio cache")
	}
	if needsImageCache && !hasImageCache {
		missing = append(missing, "image cache")
	}
	if record.Tokens.Image > 0 && rates.Image == 0 {
		missing = append(missing, "image")
	}
	if len(missing) > 0 {
		return rates, resolved, errors.Wrapf(ErrRealtimePriceUnavailable, "%s for model %q", strings.Join(missing, ", "), modelName)
	}
	return rates, resolved, nil
}

// realtimeSupplement returns audio-cache/input and image/text input multipliers
// missing from the legacy media pricing schema. Sources verified 2026-09-09:
// https://developers.openai.com/api/docs/pricing and the corresponding model pages.
// Text/audio base prices continue to come from the existing pricing resolver.
// Dated snapshots use their family's modality discounts, not a guessed new price.
func realtimeSupplement(modelName string) (audioCacheDiscount, imageMultiplier float64, ok bool) {
	name := modelName
	if len(name) > 11 && name[len(name)-11] == '-' {
		if _, err := time.Parse("2006-01-02", name[len(name)-10:]); err == nil {
			name = name[:len(name)-11]
		}
	}
	switch name {
	case "gpt-realtime", "gpt-realtime-1.5", "gpt-realtime-2", "gpt-realtime-2.1":
		return 0.4 / 32.0, 5.0 / 4.0, true
	case "gpt-realtime-mini", "gpt-realtime-2.1-mini":
		return 0.3 / 10.0, 0.8 / 0.6, true
	case "gpt-4o-realtime-preview":
		return 2.5 / 40.0, 0, true
	case "gpt-4o-mini-realtime-preview":
		return 0.3 / 10.0, 0, true
	default:
		return 0, 0, false
	}
}
