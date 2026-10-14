package openai

import (
	"maps"
	"slices"

	"github.com/Laisky/one-api/relay/adaptor"
)

// openAIOctoberSnapshots contains only snapshot IDs explicitly published on
// their model cards, not dates inferred from launch announcements.
// Sources reviewed 2026-10-13 America/Toronto (2026-10-14 UTC):
//   - https://developers.openai.com/api/docs/models/gpt-5.4-pro
//   - https://developers.openai.com/api/docs/models/gpt-5.5-pro
//   - https://developers.openai.com/api/docs/models/gpt-4o-mini-transcribe
var openAIOctoberSnapshots = map[string]string{
	"gpt-5.4-pro-2026-03-05":            "gpt-5.4-pro",
	"gpt-5.5-pro-2026-04-23":            "gpt-5.5-pro",
	"gpt-4o-mini-transcribe-2025-03-20": "gpt-4o-mini-transcribe",
	"gpt-4o-mini-transcribe-2025-12-15": "gpt-4o-mini-transcribe",
}

// applyOpenAIModelCatalog202610 returns a catalog with October's verified model
// metadata and missing snapshots. It does not mutate its input or change any
// existing tariff, channel mapping, endpoint policy, or service-tier billing.
func applyOpenAIModelCatalog202610(models map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	updated := maps.Clone(models)
	if updated == nil {
		updated = make(map[string]adaptor.ModelConfig)
	}

	// These Pro models do not accept the full non-Pro effort ladder. The
	// shared request normalization already consumes these exact metadata.
	for name, defaultEffort := range map[string]string{
		"gpt-5.4-pro": "medium",
		"gpt-5.5-pro": "high",
	} {
		cfg, ok := updated[name]
		if !ok {
			continue
		}
		cfg = cfg.Clone()
		cfg.SupportedReasoningEfforts = []string{"medium", "high", "xhigh"}
		cfg.DefaultReasoningEffort = defaultEffort
		if name == "gpt-5.4-pro" {
			// The model card explicitly marks structured outputs unsupported.
			cfg.SupportedFeatures = slices.DeleteFunc(cfg.SupportedFeatures, func(feature string) bool {
				return feature == "structured_outputs"
			})
		}
		updated[name] = cfg
	}

	// The October 7 changelog updates this rolling alias without identifying
	// it as GPT-5.5. Its model card retains 400K/128K and $5/$0.50/$30 rates.
	// https://developers.openai.com/api/docs/models/chat-latest
	// https://developers.openai.com/api/docs/changelog
	if cfg, ok := updated["chat-latest"]; ok {
		cfg = cfg.Clone()
		cfg.Description = "chat-latest: rolling alias for the latest Instant model used in ChatGPT; snapshot updated 2026-10-07. OpenAI recommends the GPT-6 family for production API use. The alias is not version-pinned."
		if !slices.Contains(cfg.SupportedFeatures, "web_search") {
			cfg.SupportedFeatures = append(cfg.SupportedFeatures, "web_search")
		}
		updated["chat-latest"] = cfg
	}

	// Clone before adding lifecycle notes: older transcription snapshots have
	// a different retirement date from the rolling alias/current snapshot.
	for snapshot, base := range openAIOctoberSnapshots {
		if _, exists := updated[snapshot]; exists {
			continue
		}
		if cfg, ok := updated[base]; ok {
			cfg = cfg.Clone()
			cfg.Description = snapshot + ": published snapshot of " + base + ". " + cfg.Description
			updated[snapshot] = cfg
		}
	}

	applyOpenAIOctoberLifecycle(updated)
	return updated
}
