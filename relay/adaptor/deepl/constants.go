package deepl

import "github.com/Laisky/one-api/relay/adaptor"

// ModelList retains compatibility aliases for common DeepL routes.
// DeepL's official API exposes language pairs and model_type choices rather than canonical model IDs,
// so these entries remain intentionally curated.

var ModelList = []string{
	"deepl-zh",
	"deepl-en",
	"deepl-ja",
}

// DeepLToolingDefaults captures that DeepL's translation API does not publish per-call tooling charges (retrieved 2026-04-28).
// Source: https://developers.deepl.com/docs/api-reference
var DeepLToolingDefaults = adaptor.ChannelToolConfig{}
