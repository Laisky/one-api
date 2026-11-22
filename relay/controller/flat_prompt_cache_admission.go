package controller

import (
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// includeFlatPromptCacheTariff reports whether a verified OpenRouter paid-write
// model must use complete cache-aware admission even when its channel override
// has no tiers. Parameters: m identifies the selected host and modelName is the
// mapped provider ID. It returns false for every other host and unverified ID,
// preserving their existing flat-price behavior. Cache intent is always read
// from the prepared payload; this predicate does not enable request controls.
func includeFlatPromptCacheTariff(m *metalib.Meta, modelName string) bool {
	return m != nil && m.ChannelType == channeltype.OpenRouter && openrouter.ImplicitPromptCacheWrites(modelName)
}
