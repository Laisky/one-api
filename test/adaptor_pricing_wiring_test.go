package test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// pricingAdaptorForChannel builds the adaptor exactly the way the billing path
// does (relay/controller.resolvePricingAdaptor), including the channel-type bind
// that OpenAI-compatible channels need in order to price themselves.
//
// Parameters:
//   - channelType: the channel type under test.
//
// Return values:
//   - adaptor.Adaptor: the provider the billing path would use, or nil.
func pricingAdaptorForChannel(channelType int) adaptor.Adaptor {
	provider := relay.GetAdaptor(channeltype.ToAPIType(channelType))
	if provider == nil {
		provider = relay.GetAdaptor(channelType)
	}
	if aware, ok := provider.(adaptor.ChannelTypeAware); ok {
		aware.SetChannelType(channelType)
	}
	return provider
}

// unpricedByDesign lists models that genuinely have no per-token price to publish.
// It is deliberately empty: every model this gateway advertises must be priced by
// the channel that serves it. Add an entry only with the reason recorded here.
var unpricedByDesign = map[string]bool{}

// TestEveryAdvertisedModelIsPricedByItsChannel is the guard for a whole class of
// silent mis-billing: a channel advertises a model, the provider cannot price it,
// and billing quietly falls back to DefaultPricingMethods' flat 2.5 USD/1M.
//
// Three separate defects produced exactly that and all three were live:
//   - the OpenAI adaptor ignored a.ChannelType, so all 13 OpenAI-compatible
//     channel types (Doubao, MiniMax, BaiduV2, SiliconFlow, ...) were unpriced —
//     Doubao-pro-32k billed ~8750x its published rate;
//   - ollama/aiproxy/palm embedded DefaultPricingMethods and never returned their
//     own ModelRatios;
//   - xunfeiv2/lingyiwanwu returned hand-written maps whose keys did not
//     intersect the model ids they advertise.
func TestEveryAdvertisedModelIsPricedByItsChannel(t *testing.T) {
	pricing.InitializeGlobalPricingManager(relay.GetAdaptor)
	now := time.Now()

	for channelType := 1; channelType < channeltype.Dummy; channelType++ {
		provider := pricingAdaptorForChannel(channelType)
		if provider == nil {
			continue
		}

		// Mirror controller/model.go: compatible channels advertise the sub-provider
		// catalog, everything else advertises the adaptor's own list.
		_, models := openai.GetCompatibleChannelMeta(channelType)
		if channeltype.ToAPIType(channelType) != 0 {
			models = provider.GetModelList()
		}
		if len(models) == 0 {
			continue
		}

		var unpriced []string
		for _, modelName := range models {
			if unpricedByDesign[modelName] {
				continue
			}
			if _, found := pricing.ResolveModelConfigRatioOnly(modelName, nil, provider, now); !found {
				unpriced = append(unpriced, modelName)
			}
		}
		sort.Strings(unpriced)
		assert.Emptyf(t, unpriced,
			"channel type %d (%s) advertises %d model(s) that no pricing layer can price, so billing "+
				"silently falls back to %.2f USD/1M: %v",
			channelType, provider.GetChannelName(), len(unpriced),
			2.5, unpriced)
	}
}

// TestCompatibleChannelPricingMatchesAdvertisedCatalog pins GetCompatibleChannelPricing
// and GetCompatibleChannelMeta to the same switch. They are two parallel lists of
// the same channel types; if one gains a case the other must too, or the new
// channel silently loses (or invents) pricing.
func TestCompatibleChannelPricingMatchesAdvertisedCatalog(t *testing.T) {
	for _, channelType := range openai.CompatibleChannels {
		name, models := openai.GetCompatibleChannelMeta(channelType)
		pricingTable := openai.GetCompatibleChannelPricing(channelType)
		require.NotEmptyf(t, pricingTable, "channel type %d (%s) has no pricing table", channelType, name)

		var missing []string
		for _, modelName := range models {
			if _, ok := pricingTable[modelName]; !ok {
				missing = append(missing, modelName)
			}
		}
		sort.Strings(missing)
		assert.Emptyf(t, missing,
			"channel type %d (%s): GetCompatibleChannelMeta advertises models absent from "+
				"GetCompatibleChannelPricing: %v", channelType, name, missing)
	}
}

// perTokenUnitErrorFloorUsdPerMillion is the lower bound for a per-token price.
//
// A per-1k price entered where the codebase expects per-1M is 1000x too low, and
// that has happened repeatedly (baiduv2 ERNIE, MiniMax abab*, legacy Doubao,
// Zhipu glm-3-turbo). Those errors land at or below 0.0007 USD per 1M tokens,
// while the cheapest genuinely-priced model in the tree is an embedding model at
// 0.002 USD per 1M. This floor sits between the two.
const perTokenUnitErrorFloorUsdPerMillion = 0.001

// TestNoPerTokenPriceLooksLikeAUnitError catches the per-1k-vs-per-1M class before
// it reaches billing. It only inspects per-token ratios; per-call, image, video and
// audio pricing use different units and are skipped.
func TestNoPerTokenPriceLooksLikeAUnitError(t *testing.T) {
	type offender struct {
		channel, model string
		usdPerMillion  float64
	}
	var offenders []offender
	seen := map[string]bool{}

	for channelType := 1; channelType < channeltype.Dummy; channelType++ {
		provider := pricingAdaptorForChannel(channelType)
		if provider == nil {
			continue
		}
		channelName := provider.GetChannelName()
		for modelName, cfg := range provider.GetDefaultModelPricing() {
			// Ratio == 0 means "not priced per token" (free tiers and per-call models).
			if cfg.Ratio <= 0 || cfg.PerCall != nil || cfg.Image != nil || cfg.Video != nil {
				continue
			}
			key := channelName + "/" + modelName
			if seen[key] {
				continue
			}
			seen[key] = true

			usdPerMillion := cfg.Ratio / billingratio.MilliTokensUsd
			if usdPerMillion < perTokenUnitErrorFloorUsdPerMillion {
				offenders = append(offenders, offender{channelName, modelName, usdPerMillion})
			}
		}
	}

	sort.Slice(offenders, func(i, j int) bool { return offenders[i].usdPerMillion < offenders[j].usdPerMillion })
	lines := make([]string, 0, len(offenders))
	for _, o := range offenders {
		lines = append(lines, fmt.Sprintf("%s/%s = %.8f USD/1M", o.channel, o.model, o.usdPerMillion))
	}
	assert.Emptyf(t, lines,
		"per-token price below %.4f USD/1M; this is what a per-1k price entered as per-1M looks like "+
			"(a 1000x under-charge). Fix the unit, or price the model per call: %v",
		perTokenUnitErrorFloorUsdPerMillion, lines)
}
