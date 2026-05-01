package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// videoModelRatios captures pricing and metadata for OpenAI Sora video models.
// Pricing sourced from OpenAI Sora preview documentation (USD per rendered second).
// Source: https://openai.com/sora.
var videoModelRatios = map[string]adaptor.ModelConfig{
	"sora-2": {
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   0.10,
			BaseResolution: "1280x720",
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "Sora 2: text-to-video model rendering 720p clips.",
	},
	"sora-2-pro": {
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   0.50,
			BaseResolution: "1920x1080",
		},
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "Sora 2 Pro: text-to-video model rendering 1080p clips.",
	},
}
