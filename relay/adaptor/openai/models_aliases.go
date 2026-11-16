package openai

import (
	"maps"

	"github.com/Laisky/one-api/relay/adaptor"
)

// openAIPublishedSnapshots contains only snapshot IDs explicitly published on
// their model cards, not dates inferred from launch announcements.
// Sources reviewed 2026-10-13 America/Toronto (2026-10-14 UTC):
//   - https://developers.openai.com/api/docs/models/gpt-5.4-pro
//   - https://developers.openai.com/api/docs/models/gpt-5.5-pro
//   - https://developers.openai.com/api/docs/models/gpt-4o-mini-transcribe
var openAIPublishedSnapshots = map[string]string{
	"gpt-5.4-pro-2026-03-05":            "gpt-5.4-pro",
	"gpt-5.5-pro-2026-04-23":            "gpt-5.5-pro",
	"gpt-4o-mini-transcribe-2025-03-20": "gpt-4o-mini-transcribe",
	"gpt-4o-mini-transcribe-2025-12-15": "gpt-4o-mini-transcribe",
}

// completeOpenAIModelCatalog returns a catalog with published snapshots and
// lifecycle annotations. It does not mutate its input or change any existing
// tariff, channel mapping, endpoint policy, or service-tier billing.
func completeOpenAIModelCatalog(models map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	updated := maps.Clone(models)
	if updated == nil {
		updated = make(map[string]adaptor.ModelConfig)
	}

	// Clone before adding lifecycle notes: older transcription snapshots have
	// a different retirement date from the rolling alias/current snapshot.
	for snapshot, base := range openAIPublishedSnapshots {
		if _, exists := updated[snapshot]; exists {
			continue
		}
		if cfg, ok := updated[base]; ok {
			cfg = cfg.Clone()
			cfg.Description = snapshot + ": published snapshot of " + base + ". " + cfg.Description
			updated[snapshot] = cfg
		}
	}

	applyOpenAILifecycle(updated)
	return updated
}
