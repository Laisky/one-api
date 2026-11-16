package xai

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// videoModels returns the complete xAI video catalog, including retained historical IDs.
// It takes no parameters and keeps the effective definitions together by capability.
// Sources: https://docs.x.ai/developers/models and https://docs.x.ai/developers/pricing
// https://docs.x.ai/developers/models/grok-imagine-video-1.5-preview
// https://docs.x.ai/developers/model-capabilities/imagine
func videoModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"grok-imagine-video": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Video: &adaptor.VideoPricingConfig{
				PerSecondUsd:   0.05,  // 480p base
				InputImageUsd:  0.002, // Verified 2026-09-22.
				BaseResolution: "480p",
				ResolutionMultipliers: map[string]float64{
					"720p": 1.4, // $0.07/sec
				},
			},
			InputModalities:  []string{"text", "image", "video"},
			OutputModalities: []string{"video"},
			Description:      "Grok Imagine Video — text-to-video generation; 480p $0.05/sec, 720p $0.07/sec.",
		}).Clone(),
		"grok-imagine-video-1.5": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Video: &adaptor.VideoPricingConfig{
				PerSecondUsd:   0.08,
				InputImageUsd:  0.01, // Verified 2026-09-22.
				BaseResolution: "480p",
				ResolutionMultipliers: map[string]float64{
					"720p":  1.75,
					"1080p": 3.125,
				},
			},
			InputModalities:  []string{"text", "image", "audio"},
			OutputModalities: []string{"video"},
			Description:      "Grok Imagine Video 1.5 accepts text, image, and preset-voice audio references and generates 480p, 720p, or 1080p video at $0.08/$0.14/$0.25 per second.",
		}).Clone(),
		"grok-imagine-video-1.5-2026-05-30": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Video: &adaptor.VideoPricingConfig{
				PerSecondUsd:   0.08,
				InputImageUsd:  0.01, // Verified 2026-09-22.
				BaseResolution: "480p",
				ResolutionMultipliers: map[string]float64{
					"720p":  1.75,
					"1080p": 3.125,
				},
			},
			InputModalities:  []string{"text", "image", "audio"},
			OutputModalities: []string{"video"},
			Description:      "Grok Imagine Video 1.5 accepts text, image, and preset-voice audio references and generates 480p, 720p, or 1080p video at $0.08/$0.14/$0.25 per second.",
		}).Clone(),
		"grok-imagine-video-1.5-preview": (adaptor.ModelConfig{
			Ratio:           0,
			CompletionRatio: 1.0,
			Video: &adaptor.VideoPricingConfig{
				PerSecondUsd:   0.08, // 480p base
				InputImageUsd:  0.01, // Verified 2026-09-22.
				BaseResolution: "480p",
				ResolutionMultipliers: map[string]float64{
					"720p":  1.75,  // $0.14/sec
					"1080p": 3.125, // $0.25/sec
				},
			},
			InputModalities:  []string{"text", "image", "audio"},
			OutputModalities: []string{"video"},
			Description:      "Grok Imagine Video 1.5 accepts text, image, and preset-voice audio references and generates 480p, 720p, or 1080p video at $0.08/$0.14/$0.25 per second.",
		}).Clone(),
	}
}
