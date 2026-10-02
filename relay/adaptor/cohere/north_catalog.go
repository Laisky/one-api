package cohere

import "github.com/Laisky/one-api/relay/adaptor"

// init registers North's published chat IDs without guessing production tariffs.
// It accepts no parameters and returns nothing. Sources verified 2026-09-24:
//   - https://docs.cohere.com/docs/north-mini-code-1.0
//   - https://docs.cohere.com/docs/north-small-translate-1.0
//
// Both cards explicitly describe free API access until rate limits, including
// production keys. Dedicated Model Vault instance prices are not token prices.
func init() {
	for id, cfg := range map[string]adaptor.ModelConfig{
		"north-mini-code-1-0": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength: 256000, MaxOutputTokens: 64000,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			Description:                 "North Mini Code 1.0, Cohere's 30B/3B-active coding model. Public API access is free until rate limits for trial and production keys; dedicated Model Vault production pricing must be configured separately. This adaptor currently supports basic text chat, not the model's native tool/reasoning controls.",
		},
		"north-small-translate-1-0": {
			Ratio: 0, CompletionRatio: 1,
			ContextLength: 16000, MaxOutputTokens: 16000,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens"},
			Description:                 "North Small Translate 1.0, Cohere's multilingual translation model. Public API access is free until rate limits for trial and production keys; production deployment contracts require separate operator pricing. Text input and output only; no inherited Command tools or vision advertisement.",
		},
	} {
		if _, exists := ModelRatios[id]; exists {
			panic("cohere: duplicate North catalog ID " + id)
		}
		ModelRatios[id] = cfg
	}
}
