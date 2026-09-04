package replicate

import (
	"github.com/Laisky/one-api/relay/adaptor"
	ratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// replicateLanguagePricingAdjustments contains temporary provider promotions
// that must not replace a model's long-lived base pricing. Each entry is merged
// into the corresponding model after the base language catalog is copied.
var replicateLanguagePricingAdjustments = map[string][]adaptor.TimeWindow{
	"openai/gpt-5.6-sol": {
		{
			Name:     "replicate-gpt-5.6-sol-50-percent-promotion",
			TimeZone: "UTC",
			DateFrom: "2026-09-04",
			// DateTo is exclusive, so 2026-09-19 keeps the published discount
			// active through all of 2026-09-18 before reverting to base pricing.
			DateTo: "2026-09-19",
			Ranges: []adaptor.ClockRange{
				{Start: "00:00", End: "00:00"},
			},
			Overlay: adaptor.ModelConfig{
				Ratio:            2.50 * ratio.MilliTokensUsd,
				CompletionRatio:  15.00 / 2.50,
				CachedInputRatio: 0.25 * ratio.MilliTokensUsd,
			},
		},
	},
}
