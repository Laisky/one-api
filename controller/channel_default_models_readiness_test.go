package controller

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/channeltype"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/meta"
)

// TestDefaultChannelModelsNeedNoOperatorPricing pins the upgrade contract for
// unconfigured admins: every model a channel type lists by default (the
// /api/models catalog the channel editor fills a new channel from) must be
// servable with empty model_configs, so saving a default channel never raises
// a pricing warning and admission never refuses a default model for missing
// operator pricing. A model whose host has no qualified tariff belongs off the
// default list (operators may still add and price it). Parameters: t is the
// test handle. Returns: none.
func TestDefaultChannelModelsNeedNoOperatorPricing(t *testing.T) {
	require.NotEmpty(t, channelId2Models, "the default model catalog must be initialized")
	types := make([]int, 0, len(channelId2Models))
	for channelType := range channelId2Models {
		types = append(types, channelType)
	}
	slices.Sort(types)
	at := time.Now().UTC()
	var gaps []string
	for _, channelType := range types {
		provider := relaycontroller.ResolveChannelPricingAdaptor(channelType)
		if provider == nil {
			continue
		}
		for _, gap := range relaycontroller.EvaluateChannelPricingReadiness(provider, channelId2Models[channelType], nil, nil, at) {
			gaps = append(gaps, fmt.Sprintf("%s(%d) %s: %s", channeltype.IdToName(channelType), channelType, gap.Model, gap.Requirement))
		}
	}
	require.Empty(t, gaps, "default channel models must not need operator pricing")

	// Control: the filter is exercised. Known operator-priced models stay in
	// their adaptor catalogs (operators can still add and price them) but are
	// kept off the default lists, and no servable model is dropped.
	for channelType, name := range map[int]string{
		channeltype.Ali:       "qwen-audio-turbo",
		channeltype.Groq:      "minimaxai/minimax-m2.7",
		channeltype.VertextAI: "claude-fable-5-2",
		channeltype.Gemini:    "gemini-3.5-transcribe-live",
	} {
		raw := rawAdaptorModelList(t, channelType)
		require.Contains(t, raw, name, "%s must remain in the adaptor catalog", name)
		require.NotContains(t, channelId2Models[channelType], name, "%s must not be a default model", name)
		provider := relaycontroller.ResolveChannelPricingAdaptor(channelType)
		refused := map[string]bool{}
		for _, gap := range relaycontroller.EvaluateChannelPricingReadiness(provider, raw, nil, nil, at) {
			refused[gap.Model] = true
		}
		for _, model := range raw {
			require.Equal(t, !refused[model], slices.Contains(channelId2Models[channelType], model),
				"%s default-list membership must equal zero-config servability", model)
		}
	}
}

// rawAdaptorModelList returns a channel type's unfiltered adaptor model list,
// built the way the catalog init builds it. Parameters: t is the test handle
// and channelType a channel type ID. Returns: the adaptor's model list.
func rawAdaptorModelList(t *testing.T, channelType int) []string {
	t.Helper()
	adaptor := relay.GetAdaptor(channeltype.ToAPIType(channelType))
	require.NotNil(t, adaptor)
	adaptor.Init(&meta.Meta{ChannelType: channelType})
	return adaptor.GetModelList()
}
