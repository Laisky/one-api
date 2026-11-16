package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// novemberModels returns independently owned Fireworks Standard serverless
// defaults reviewed on 2026-11-16. It takes no parameters. Sources:
// https://docs.fireworks.ai/serverless/pricing
// https://fireworks.ai/models?featured=true&serverless=true
// https://fireworks.ai/models/fireworks/kimi-k3p1-pro
// https://fireworks.ai/models/fireworks/minimax-m3p1-flash
// https://fireworks.ai/models/deepseek-ai/deepseek-v4p1-flash-1017
// https://fireworks.ai/models/fireworks/muse-spark-open
// https://fireworks.ai/models/fireworks/ultramarine-1-flash-us
func novemberModels() map[string]adaptor.ModelConfig {
	models := make(map[string]adaptor.ModelConfig, 5)
	for _, entry := range []struct {
		id                    string
		input, cached, output float64
		tools, reasoning      bool
		description           string
	}{
		{"kimi-k3p1-pro", 2, .20, 10, true, true, "Fireworks-hosted Kimi K3.1 Pro with native text/image input and 1M context."},
		{"minimax-m3p1-flash", .15, .015, .60, true, true, "Fireworks-hosted MiniMax M3.1 Flash with native text/image input and 1M context. The card describes always-on reasoning with adjustable effort, but does not enumerate the hosted effort vocabulary or default."},
		{"deepseek-v4p1-flash-1017", .30, .006, 1.20, true, true, "Fireworks-hosted DeepSeek V4.1 Flash 1017 with native text/image input and 1M context. The previous unversioned Fireworks ID remains independently configured."},
		{"muse-spark-open", 1.25, .15, 4.25, true, true, "Fireworks-hosted Muse Spark Open with text/image input and 1M context. US-only capacity uses the published Standard tariff because the normal US premium is explicitly waived."},
		{"ultramarine-1-flash-us", .15, .03, .50, false, false, "Fireworks-hosted Ultramarine 1 Flash LoRA with text/image input and 1M context. On 2026-11-16 both the model card and central table agree on 0.15/0.03/0.50 USD/M input/cache/output. The current card describes global capacity despite the -us ID and explicitly does not support function calling; no base-GLM tool or effort metadata is inherited."},
	} {
		cfg := adaptor.ModelConfig{
			Ratio:            entry.input * ratio.MilliTokensUsd,
			CachedInputRatio: entry.cached * ratio.MilliTokensUsd,
			CompletionRatio:  entry.output / entry.input,
			ContextLength:    1_048_576,
			InputModalities:  []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop"},
			Description:                 entry.description + " Maximum output and hosted precision are unspecified. Priority, Fast, Ultra-fast and deployment-specific charges are separate operator configuration.",
		}
		if entry.tools {
			cfg.SupportedFeatures = append(cfg.SupportedFeatures, "tools")
			cfg.SupportedSamplingParameters = append(cfg.SupportedSamplingParameters, "tools", "tool_choice")
		}
		if entry.reasoning {
			cfg.SupportedFeatures = append(cfg.SupportedFeatures, "reasoning")
		}
		if entry.id == "ultramarine-1-flash-us" {
			// The card rounds its context label to 1040k; do not infer an exact
			// token ceiling from the base GLM checkpoint or another endpoint.
			cfg.ContextLength = 0
			cfg.Description += " The card publishes an approximate 1M context; the exact token ceiling is unconfirmed."
		}
		models["accounts/fireworks/models/"+entry.id] = cfg
	}
	return models
}
