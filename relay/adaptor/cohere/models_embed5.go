package cohere

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init registers source-verified Embed 5 defaults without changing legacy model
// prices. It accepts no parameters and returns no values. Sources, 2026-11-16:
// https://docs.cohere.com/docs/models
// https://docs.cohere.com/docs/cohere-embed
// https://cohere.com/blog/embed-5
func init() {
	models := make(map[string]adaptor.ModelConfig, 2)
	for id, textUSD := range map[string]float64{"embed-v5.0-pro": 0.12, "embed-v5.0-fast": 0.08} {
		models[id] = adaptor.ModelConfig{
			Ratio: textUSD * ratio.MilliTokensUsd, CompletionRatio: 1,
			ContextLength:   128_000,
			InputModalities: []string{"text"},
			Embedding: &adaptor.EmbeddingPricingConfig{
				TextTokenRatio:  textUSD * ratio.MilliTokensUsd,
				ImageTokenRatio: 0.40 * ratio.MilliTokensUsd,
			},
			Description: "Cohere Embed 5 via /v2/embed, 128k context, configurable dimensions 256/512/768/1024/1536/2048 (default 2048). This adaptor admits text input only; the documented $0.40/M image-token tariff does not imply support for the provider's native media payload. Text billing uses meta.billed_units.input_tokens.",
		}
	}
	ModelRatios = adaptor.JoinModelCatalogs(ModelRatios, models)
}
