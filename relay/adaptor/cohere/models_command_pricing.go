package cohere

// init replaces historical estimated tariffs with Cohere's explicitly free
// rate-limited API tariffs. It accepts no parameters and returns no values.
// Dedicated Model Vault and separately negotiated production prices are not
// token tariffs and must still be configured by the channel operator.
// Sources verified 2026-10-12:
//   - https://docs.cohere.com/docs/command-a-plus
//   - https://docs.cohere.com/docs/command-a-reasoning
//   - https://docs.cohere.com/docs/command-a-translate
//   - https://docs.cohere.com/docs/command-a-vision
func init() {
	for id, description := range map[string]string{
		"command-a-plus-05-2026":      "Command A+ (May 2026), Cohere's multimodal mixture-of-experts model with 128K context and 64K maximum output.",
		"command-a-reasoning-08-2025": "Command A Reasoning (August 2025), Cohere's hybrid reasoning model with 256K context.",
		"command-a-translate-08-2025": "Command A Translate (August 2025), Cohere's multilingual translation specialist.",
		"command-a-vision-07-2025":    "Command A Vision (July 2025), Cohere's image-understanding model with 128K context; tool use is not supported.",
	} {
		cfg, exists := ModelRatios[id]
		if !exists {
			panic("cohere: missing Command model for verified API tariff " + id)
		}
		cfg = cfg.Clone()
		cfg.Ratio = 0
		cfg.CompletionRatio = 1
		cfg.Description = description + " Public API access is free until rate limits for both trial and production keys. Dedicated Model Vault or negotiated production deployment prices must be configured separately; this is not unlimited free production hosting."
		ModelRatios[id] = cfg
	}
}
