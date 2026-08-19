package aiproxy

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// ModelRatios uses the same pricing as OpenAI since AIProxy is an OpenAI proxy
var ModelRatios = openai.ModelRatios

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// AIProxyToolingDefaults mirrors OpenAI's built-in tooling defaults since AIProxy forwards to OpenAI.
var AIProxyToolingDefaults = openai.OpenAIToolingDefaults
