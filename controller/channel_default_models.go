package controller

import (
	"time"

	relaycontroller "github.com/Laisky/one-api/relay/controller"
)

// zeroConfigServableModels returns the models a new channel of channelType may
// be filled with by default: the adaptor's model list minus every model request
// admission would refuse while the channel's model_configs are empty (models
// the host serves without a qualified tariff, or Live-only models whose host
// requires operator realtime prices). Such models stay in the catalog, so an
// operator can still add and price them; they are only kept out of the default
// list, so an unconfigured admin never fills, resets, or saves a channel into
// pricing warnings or refused requests. Parameters: channelType is a channel
// type ID and models its adaptor's model list. Returns: the filtered list in
// the original order; models itself when nothing is filtered.
func zeroConfigServableModels(channelType int, models []string) []string {
	provider := relaycontroller.ResolveChannelPricingAdaptor(channelType)
	if provider == nil || len(models) == 0 {
		return models
	}
	gaps := relaycontroller.EvaluateChannelPricingReadiness(provider, models, nil, nil, time.Now().UTC())
	if len(gaps) == 0 {
		return models
	}
	refused := make(map[string]struct{}, len(gaps))
	for _, gap := range gaps {
		refused[gap.Model] = struct{}{}
	}
	servable := make([]string, 0, len(models)-len(refused))
	for _, name := range models {
		if _, ok := refused[name]; !ok {
			servable = append(servable, name)
		}
	}
	return servable
}
