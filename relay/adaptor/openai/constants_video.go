package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// videoModelRatios captures pricing and metadata for OpenAI Sora video models.
// Pricing sourced from OpenAI Sora 2 / Sora 2 Pro documentation (USD per rendered second).
// Sources verified 2026-05-19:
//   - https://developers.openai.com/api/docs/models/sora-2 ($0.10/sec at 720p)
//   - https://developers.openai.com/api/docs/models/sora-2-pro (tiered: $0.30/$0.50/$0.70 per second at 720p/1024p/1080p)
var videoModelRatios = map[string]adaptor.ModelConfig{
	"sora-2": {
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   0.10,
			BaseResolution: "1280x720",
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"video"},
		Description:      "Sora 2: text-to-video model rendering 720p clips at $0.10/second.",
	},
	"sora-2-pro": {
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   0.30,
			BaseResolution: "1280x720",
			ResolutionMultipliers: map[string]float64{
				"1280x720":  1,           // $0.30/sec
				"1792x1024": 0.50 / 0.30, // $0.50/sec
				"1920x1080": 0.70 / 0.30, // $0.70/sec
			},
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"video"},
		Description:      "Sora 2 Pro: text-to-video model with tiered resolutions ($0.30/$0.50/$0.70 per sec at 720p/1024p/1080p).",
	},
}
