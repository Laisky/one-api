package quota

import (
	"fmt"
	"math"
	"time"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/realtime"
)

// computeRealtime prices each authoritative receipt under its own model and
// context tier, then rounds the session once. It never recursively prices the
// session aggregates as text, nor charges transcription at the conversation rate.
func computeRealtime(input ComputeInput) ComputeResult {
	ledger := input.Usage.Realtime
	result := ComputeResult{PromptTokens: int(ledger.InputTokens), CompletionTokens: int(ledger.OutputTokens),
		UsedModelRatio: input.ModelRatio, BillingIssues: append([]string(nil), ledger.Issues...)}
	var total, correction float64
	for _, record := range ledger.Records {
		rates, ratios, err := resolveRealtimeRecordRates(input, record)
		if err != nil {
			if len(result.BillingIssues) < 16 {
				result.BillingIssues = append(result.BillingIssues, err.Error())
			}
			continue
		}
		if record.Model == "" {
			result.UsedModelRatio, result.UsedCompletionRatio = ratios.UsedModelRatio, ratios.UsedCompletionRatio
		}
		cost, err := realtime.Cost(record, rates)
		if err != nil {
			if len(result.BillingIssues) < 16 {
				result.BillingIssues = append(result.BillingIssues, err.Error())
			}
			continue
		}
		// Compensated summation avoids per-turn floating-point accumulation drift.
		y := cost - correction
		next := total + y
		correction = (next - total) - y
		total = next
		t := record.Tokens
		result.CachedPromptTokens += int(t.CachedText + t.CachedAudio + t.CachedImage)
	}
	quota, err := realtime.RoundQuota(total, input.GroupRatio, input.Usage.ToolsCost)
	if err != nil {
		result.BillingIssues = append(result.BillingIssues, err.Error())
		return result
	}
	result.TotalQuota = quota
	return result
}

// resolveRealtimeRecordRates resolves existing channel, provider/global, tier,
// time-window and legacy scalar settings for one receipt. Supplemental cache
// discounts are relative to the configured modality price, so existing audio
// markups remain effective without introducing a second absolute base-price table.
func resolveRealtimeRecordRates(input ComputeInput, record realtime.Record) (realtime.Rates, ComputeResult, error) {
	modelName := input.ModelName
	if record.Model != "" {
		modelName = record.Model
	}
	cfg, known := pricing.ResolveModelConfigRatioOnly(modelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime)
	if !known {
		return realtime.Rates{}, ComputeResult{}, fmt.Errorf("realtime pricing unavailable for model %q", modelName)
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
			return rates, resolved, fmt.Errorf("realtime duration pricing unavailable for model %q", modelName)
		}
		rates.Second = audio.UsdPerSecond * billingratio.QuotaPerUsd
		return rates, resolved, nil
	}
	if rates.Text == 0 {
		rates.CachedText = 0
	}
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
		return rates, resolved, fmt.Errorf("realtime audio pricing unavailable for model %q", modelName)
	}
	cachedAudioDiscount, imageMultiplier, hasSupplement := realtimeSupplement(modelName)
	if hasSupplement {
		rates.CachedAudio = rates.Audio * cachedAudioDiscount
		rates.Image = rates.Text * imageMultiplier
		rates.CachedImage = rates.Image * 0.1
	}
	if image, ok := pricing.ResolveImagePricing(modelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime); ok && image.PromptRatio > 0 {
		rates.Image = rates.Text * image.PromptRatio
		rates.CachedImage = rates.Image * 0.1
	}
	if (record.Tokens.CachedAudio > 0 || record.Tokens.CachedImage > 0) && !hasSupplement {
		return rates, resolved, fmt.Errorf("realtime modality cache price unavailable for model %q", modelName)
	}
	if record.Tokens.Image > 0 && rates.Image == 0 && rates.Text != 0 {
		return rates, resolved, fmt.Errorf("realtime image price unavailable for model %q", modelName)
	}
	if math.IsNaN(rates.Text) {
		return rates, resolved, fmt.Errorf("invalid realtime model price")
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
