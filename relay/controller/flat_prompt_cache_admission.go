package controller

import (
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// includeFlatPromptCacheTariff reports whether a host's verified automatic
// paid-write contract requires complete cache-aware admission even when the
// channel override has no tiers. Parameters: m identifies the selected host and
// modelName is the mapped provider ID. It returns true for native OpenAI
// GPT-5.6+ models whose implicit 30m writes are metered, and for qualified
// OpenRouter models whose automatic writes OpenRouter bills separately. Every
// other host and unverified ID keeps its existing flat-price behavior. Cache
// intent is always read from the prepared payload; this predicate does not
// enable request controls, and a flat override stays flat because admission
// prices the resolved channel tariff without restoring provider tiers.
func includeFlatPromptCacheTariff(m *metalib.Meta, modelName string) bool {
	if m == nil {
		return false
	}
	if openai.ImplicitPromptCacheWrites(m.ChannelType, modelName) {
		return true
	}
	return m.ChannelType == channeltype.OpenRouter && openrouter.ImplicitPromptCacheWrites(modelName)
}
