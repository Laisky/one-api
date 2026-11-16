package aiproxy

import (
	"maps"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// ModelRatios preserves historical OpenAI-derived defaults for the legacy
// AIProxy library API. New native OpenAI models require independent host evidence.
var ModelRatios = legacyLibraryModelRatios()

// legacyLibraryModelRatios returns historical library defaults without the three
// native-only additions from the 2026-11-16 audit. It takes no arguments and
// never mutates OpenAI's catalog. No current contract could be retrieved from
// the adaptor's original source: https://docs.aiproxy.io/dev/library
func legacyLibraryModelRatios() map[string]adaptor.ModelConfig {
	models := maps.Clone(openai.ModelRatios)
	delete(models, "gpt-6-cyber")
	delete(models, "gpt-image-2.6-sunburst")
	delete(models, "gpt-image-2.6-flare")
	return models
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// AIProxyToolingDefaults mirrors OpenAI's built-in tooling defaults since AIProxy forwards to OpenAI.
var AIProxyToolingDefaults = openai.OpenAIToolingDefaults
