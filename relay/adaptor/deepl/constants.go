package deepl

import "github.com/Laisky/one-api/relay/adaptor"

// https://developers.deepl.com/docs/api-reference/glossaries

var ModelList = []string{
	"deepl-zh",
	"deepl-en",
	"deepl-ja",
}

// DeepLToolingDefaults captures that DeepL's translation API does not publish per-call tooling charges (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://developers.deepl.com/docs/api-reference
var DeepLToolingDefaults = adaptor.ChannelToolConfig{}
