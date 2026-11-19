package aiproxy

import "github.com/Laisky/one-api/relay/adaptor/openai"

// AIProxyToolingDefaults mirrors OpenAI's built-in tooling defaults since AIProxy forwards to OpenAI.
var AIProxyToolingDefaults = openai.OpenAIToolingDefaults
