package controller

import (
	"github.com/songquanpeng/one-api/relay"
	relayadaptor "github.com/songquanpeng/one-api/relay/adaptor"
	metalib "github.com/songquanpeng/one-api/relay/meta"
)

// resolvePricingAdaptor resolves the pricing adaptor for a request.
// Parameters: meta provides APIType and ChannelType identifiers.
// Returns: the resolved adaptor, or nil if neither type maps to a known adaptor.
func resolvePricingAdaptor(meta *metalib.Meta) relayadaptor.Adaptor {
	if meta == nil {
		return nil
	}

	if adaptor := relay.GetAdaptor(meta.APIType); adaptor != nil {
		return adaptor
	}

	return relay.GetAdaptor(meta.ChannelType)
}
