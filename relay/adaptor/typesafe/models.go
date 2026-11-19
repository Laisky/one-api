package typesafe

import (
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// modelPricing records TypeSafe's published Jev catalog checked on 2026-11-16.
// Source: https://docs.typesafe.ai/models
// Jev 1.14.0 was released on 2026-10-14. Both stable and preview aliases now
// target 1.14.0; aliases can change independently.
// $42/B input tokens = $0.042/M input tokens; output tokens are free.
var modelPricing = func() map[string]adaptor.ModelConfig {
	versions := []struct {
		id, selection string
	}{
		{"jev-1.13.0", "Pinned Jev 1.13 release."},
		{"jev-1.14.0", "Pinned Jev 1.14 release."},
		{"jev-latest", "Mutable stable alias; documented target: jev-1.14.0."},
		{"jev-preview", "Mutable preview alias; documented target: jev-1.14.0."},
	}
	models := make(map[string]adaptor.ModelConfig, len(versions))
	for _, version := range versions {
		models[version.id] = adaptor.ModelConfig{
			Ratio:            0.042 * billingratio.MilliTokensUsd,
			CompletionRatio:  0,
			ContextLength:    64000,
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
			Description: "TypeSafe System One typed evaluation (noul/choice/score), not chat. " +
				"64k state+all questions; 32k state+longest question. Input tokens billed; output free. " +
				version.selection,
		}
	}
	return models
}()
