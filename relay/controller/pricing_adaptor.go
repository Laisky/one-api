package controller

import (
	"github.com/Laisky/one-api/relay"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// resolvePricingAdaptor resolves the pricing adaptor for a request.
// Parameters: meta provides APIType and ChannelType identifiers.
// Returns: the resolved adaptor, or nil if neither type maps to a known adaptor.
func resolvePricingAdaptor(meta *metalib.Meta) relayadaptor.Adaptor {
	if meta == nil {
		return nil
	}

	resolved := relay.GetAdaptor(meta.APIType)
	if resolved == nil {
		resolved = relay.GetAdaptor(meta.ChannelType)
	}

	// The OpenAI adaptor serves every OpenAI-compatible channel, so it must be told
	// which one before it can price the request. Without this the adaptor answers
	// with OpenAI's table and every Doubao/MiniMax/BaiduV2/... model misses,
	// silently falling back to DefaultPricingMethods' 2.5 USD/1M.
	if aware, ok := resolved.(relayadaptor.ChannelTypeAware); ok {
		aware.SetChannelType(meta.ChannelType)
	}

	return resolved
}
