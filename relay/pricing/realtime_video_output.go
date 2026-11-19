package pricing

import (
	"math"
	"time"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
)

// ResolveRealtimeVideoOutputRatio resolves the price of realtime VIDEO output
// tokens (a Gemini Live avatar) as a multiple of the effective text input
// ratio, from video.output_token_ratio. It uses the same precedence as
// ResolveAudioPricing: the channel override, then, unless the provider
// declares an operator-only tariff, the provider defaults and the global
// catalog, each after its request-time window is applied.
//
// Parameters: modelName is the billed model, channelConfigs the channel's
// model_configs, provider the pricing adaptor (nil allowed), and at the request
// start time. Returns: the ratio and true when a layer sets a positive finite
// value; otherwise zero and false, which leaves video output unpriced.
func ResolveRealtimeVideoOutputRatio(modelName string, channelConfigs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (float64, bool) {
	if local, ok := channelConfigs[modelName]; ok {
		if ratio, ok := realtimeVideoOutputRatio(ApplyTimeWindow(convertLocalModelConfig(local), at)); ok {
			return ratio, true
		}
	}
	if !providerTariffEligible(provider, modelName) {
		return 0, false
	}
	if provider != nil {
		if cfg, ok := provider.GetDefaultModelPricing()[modelName]; ok {
			if ratio, ok := realtimeVideoOutputRatio(ApplyTimeWindow(cloneModelConfig(cfg), at)); ok {
				return ratio, true
			}
		}
	}
	if cfg, ok := GetGlobalModelConfig(modelName); ok {
		return realtimeVideoOutputRatio(ApplyTimeWindow(cfg, at))
	}
	return 0, false
}

// realtimeVideoOutputRatio extracts a usable video output ratio from cfg.
// Parameters: cfg is a time-window-applied model configuration. Returns: the
// ratio and true only when it is positive and finite, so a corrupt stored value
// can never become a zero or negative charge.
func realtimeVideoOutputRatio(cfg adaptor.ModelConfig) (float64, bool) {
	if cfg.Video == nil {
		return 0, false
	}
	ratio := cfg.Video.OutputTokenRatio
	if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return 0, false
	}
	return ratio, true
}
