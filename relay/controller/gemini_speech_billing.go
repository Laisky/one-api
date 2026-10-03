package controller

import (
	"math"
	"math/big"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/gemini/tts"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// geminiSpeechPrices freezes request-time channel configuration and pricing precedence.
type geminiSpeechPrices struct {
	input         quota.ComputeInput
	config        adaptor.ModelConfig
	audio         *adaptor.AudioPricingConfig
	explicitAudio bool
}

// loadGeminiSpeechPrices resolves the selected channel's token prices, not character tariffs.
func loadGeminiSpeechPrices(c *gin.Context, m *meta.Meta) (*geminiSpeechPrices, error) {
	ctx := gmw.Ctx(c)
	input := quota.ComputeInput{ModelName: m.ActualModelName, GroupRatio: c.GetFloat64(ctxkey.ChannelRatio), PricingAdaptor: resolvePricingAdaptor(m), RequestTime: m.StartTime}
	if value, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := value.(*dbmodel.Channel); ok {
			input.ChannelModelConfigs = channel.GetModelPriceConfigsWithContext(ctx)
			input.ChannelModelRatio = channel.GetModelRatioFromConfigsWithContext(ctx)
		}
	}
	if m.ChannelType == channeltype.VertextAI {
		cfg, exists := input.ChannelModelConfigs[input.ModelName]
		if !exists || cfg.Ratio <= 0 || (cfg.CompletionRatio <= 0 && (cfg.Audio == nil || cfg.Audio.CompletionRatio <= 0)) {
			return nil, errors.New("Vertex speech requires explicit channel input and output token prices; Developer API promotions are not Vertex tariffs")
		}
	}
	input.ModelRatio = pricing.ResolveModelRatioAt(input.ModelName, input.ChannelModelConfigs, input.ChannelModelRatio, input.PricingAdaptor, input.RequestTime)
	return newGeminiSpeechPrices(input)
}

// newGeminiSpeechPrices validates all rates before dispatch and preserves explicit audio overrides.
func newGeminiSpeechPrices(input quota.ComputeInput) (*geminiSpeechPrices, error) {
	// The ratio-only resolver intentionally removes Audio, including window overlays.
	// Keep the full effective local config to distinguish an explicit audio tariff
	// from provider fallback; both reservation and settlement use that decision.
	cfg, ok := pricing.ResolveModelConfig(input.ModelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime)
	if !ok {
		return nil, errors.New("Gemini speech token pricing is unavailable")
	}
	audio, _ := pricing.ResolveAudioPricing(input.ModelName, input.ChannelModelConfigs, input.PricingAdaptor, input.RequestTime)
	if audio != nil && (audio.InputUnit != "" || audio.UsdPerSecond != 0) {
		return nil, errors.New("Gemini speech requires token pricing, not a character or duration tariff")
	}
	values := []float64{input.ModelRatio, input.GroupRatio, cfg.Ratio, cfg.CompletionRatio}
	if audio != nil {
		values = append(values, audio.PromptRatio, audio.CompletionRatio)
	}
	cachedValues := []float64{cfg.CachedInputRatio}
	for _, tier := range cfg.Tiers {
		values = append(values, tier.Ratio, tier.CompletionRatio)
		cachedValues = append(cachedValues, tier.CachedInputRatio)
	}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return nil, errors.New("invalid Gemini speech token rate")
		}
	}
	for _, value := range cachedValues {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("invalid Gemini speech cache rate")
		}
	}
	_, exists := input.ChannelModelConfigs[input.ModelName]
	return &geminiSpeechPrices{input: input, config: cfg, audio: audio, explicitAudio: exists && cfg.Audio != nil && cfg.Audio.HasData()}, nil
}

// outputFactors returns unrounded factors for an audio completion token.
func (p *geminiSpeechPrices) outputFactors(base, completion float64) []float64 {
	if p.explicitAudio && p.audio != nil {
		prompt, output := p.audio.PromptRatio, p.audio.CompletionRatio
		if prompt == 0 {
			prompt = 1
		}
		if output == 0 {
			output = completion
		}
		return []float64{base, prompt, output}
	}
	// Both implemented TTS catalog entries encode audio-only output in CompletionRatio.
	// This also preserves legacy scalar/tier completion overrides when Audio is absent.
	return []float64{base, completion}
}

// charge prices text, cache hits and audio output once, using the same ratios as realtime.
func (p *geminiSpeechPrices) charge(receipt tts.Receipt) (int64, error) {
	if receipt.PromptTokens < 0 || receipt.PromptTokens > tts.MaxInputTokens || receipt.OutputTokens < 0 || receipt.OutputTokens > tts.MaxOutputTokens || receipt.CachedTokens < 0 || receipt.CachedTokens > receipt.PromptTokens {
		return 0, errors.New("invalid Gemini speech billing receipt")
	}
	probe := p.input
	probe.GroupRatio = 1
	probe.Usage = &relaymodel.Usage{PromptTokens: receipt.PromptTokens, CompletionTokens: receipt.OutputTokens}
	resolved := quota.Compute(probe)
	effective := pricing.ResolveEffectivePricingForUsageFromConfig(receipt.PromptTokens, receipt.OutputTokens, p.config)
	cached := resolved.UsedModelRatio
	if effective.CachedInputRatio < 0 {
		cached = 0
	} else if effective.CachedInputRatio > 0 {
		cached = effective.CachedInputRatio
	}
	audio := append([]float64{float64(receipt.OutputTokens), p.input.GroupRatio}, p.outputFactors(resolved.UsedModelRatio, resolved.UsedCompletionRatio)...)
	return sumGeminiSpeechTerms(
		[]float64{float64(receipt.PromptTokens - receipt.CachedTokens), resolved.UsedModelRatio, p.input.GroupRatio},
		[]float64{float64(receipt.CachedTokens), cached, p.input.GroupRatio}, audio)
}

// reserve bounds every configured tier, including cache rates above normal input rates.
// It is a temporary hold, not the final charge, and protects even non-monotonic tiers.
func (p *geminiSpeechPrices) reserve(outputLimit int) (int64, error) {
	base := max(p.input.ModelRatio, p.config.Ratio)
	completion := p.config.CompletionRatio
	cache := max(p.config.CachedInputRatio, 0)
	probe := p.input
	probe.GroupRatio = 1
	probe.Usage = &relaymodel.Usage{PromptTokens: tts.MaxInputTokens, CompletionTokens: outputLimit}
	resolved := quota.Compute(probe)
	base, completion = max(base, resolved.UsedModelRatio), max(completion, resolved.UsedCompletionRatio)
	for _, tier := range p.config.Tiers {
		base = max(base, tier.Ratio)
		completion = max(completion, tier.CompletionRatio)
		cache = max(cache, tier.CachedInputRatio)
	}
	audio := append([]float64{float64(outputLimit), p.input.GroupRatio}, p.outputFactors(base, completion)...)
	return sumGeminiSpeechTerms([]float64{tts.MaxInputTokens, max(base, cache), p.input.GroupRatio}, audio)
}

// sumGeminiSpeechTerms multiplies decimal factors, sums buckets and rounds upward once.
func sumGeminiSpeechTerms(terms ...[]float64) (int64, error) {
	total := new(big.Rat)
	for _, factors := range terms {
		term := new(big.Rat).SetInt64(1)
		for _, factor := range factors {
			if math.IsNaN(factor) || math.IsInf(factor, 0) || factor < 0 {
				return 0, errors.New("invalid speech billing factor")
			}
			rational, ok := new(big.Rat).SetString(strconv.FormatFloat(factor, 'f', -1, 64))
			if !ok {
				return 0, errors.New("invalid speech billing decimal")
			}
			term.Mul(term, rational)
		}
		total.Add(total, term)
	}
	result, remainder := new(big.Int), new(big.Int)
	result.QuoRem(total.Num(), total.Denom(), remainder)
	if remainder.Sign() != 0 {
		result.Add(result, big.NewInt(1))
	}
	if !result.IsInt64() {
		return 0, errors.New("speech charge exceeds supported quota range")
	}
	return result.Int64(), nil
}
