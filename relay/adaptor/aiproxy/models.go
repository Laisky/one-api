package aiproxy

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// ModelRatios preserves historical OpenAI-derived defaults for the legacy
// AIProxy library API. Native-only OpenAI models (openai.IsNativeOnlyModel)
// require independent host evidence; no current contract could be retrieved
// from the adaptor's original source: https://docs.aiproxy.io/dev/library
var ModelRatios = openai.InheritedModelRatios()

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)
