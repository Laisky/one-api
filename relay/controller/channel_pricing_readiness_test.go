package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// readinessAt is the evaluation time of every readiness test.
var readinessAt = time.Date(2026, time.November, 18, 12, 0, 0, 0, time.UTC)

// completeOperatorTariff is a complete operator token tariff, cache prices
// included, so no declared provider requirement is left unpriced.
var completeOperatorTariff = model.ModelConfigLocal{Ratio: 2, CompletionRatio: 6, CachedInputRatio: 0.5, CacheWrite5mRatio: 2.5, CacheWrite1hRatio: 4}

// readinessGapKeys reduces gaps to "model>upstream:requirement" keys so one
// assertion pins every field that drives the editor's warning.
func readinessGapKeys(gaps []ChannelPricingGap) []string {
	keys := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		keys = append(keys, gap.Model+">"+gap.UpstreamModel+":"+gap.Requirement)
	}
	return keys
}

// TestEvaluateChannelPricingReadinessCases pins the editor warning for the two
// refusal classes the PR #624 review named: strict GPT Image IDs without a
// per-image reservation/fallback, and operator-tariff models without a complete
// channel tariff, through aliases, alongside ordinarily priced models.
func TestEvaluateChannelPricingReadinessCases(t *testing.T) {
	imageFallback := model.ModelConfigLocal{Image: &model.ImagePricingLocal{PricePerImageUsd: 0.04}}
	for _, tc := range []struct {
		name        string
		channelType int
		models      []string
		mapping     map[string]string
		configs     map[string]model.ModelConfigLocal
		want        []string
	}{
		{name: "openai_strict_image_unpriced", channelType: channeltype.OpenAI,
			models: []string{"gpt-image-2.5-sunburst", "gpt-4o"},
			want:   []string{"gpt-image-2.5-sunburst>gpt-image-2.5-sunburst:" + PricingRequirementImageFallback}},
		{name: "openai_strict_image_priced", channelType: channeltype.OpenAI,
			models:  []string{"gpt-image-2.5-sunburst"},
			configs: map[string]model.ModelConfigLocal{"gpt-image-2.5-sunburst": imageFallback}},
		{name: "openai_strict_image_request_defaults_only", channelType: channeltype.OpenAI,
			models:  []string{"gpt-image-2.6-flare"},
			configs: map[string]model.ModelConfigLocal{"gpt-image-2.6-flare": {Image: &model.ImagePricingLocal{DefaultSize: "1024x1024", MaxImages: 2}}},
			want:    []string{"gpt-image-2.6-flare>gpt-image-2.6-flare:" + PricingRequirementImageFallback}},
		{name: "openai_alias_to_strict_image", channelType: channeltype.OpenAI,
			models:  []string{"house-image"},
			mapping: map[string]string{"house-image": "gpt-image-2.5-flare"},
			want:    []string{"house-image>gpt-image-2.5-flare:" + PricingRequirementImageFallback}},
		{name: "openai_alias_priced_under_upstream_name", channelType: channeltype.OpenAI,
			models:  []string{"house-image"},
			mapping: map[string]string{"house-image": "gpt-image-2.5-flare"},
			configs: map[string]model.ModelConfigLocal{"gpt-image-2.5-flare": imageFallback}},
		{name: "openai_alias_priced_under_alias_name_only", channelType: channeltype.OpenAI,
			models:  []string{"house-image"},
			mapping: map[string]string{"house-image": "gpt-image-2.5-flare"},
			configs: map[string]model.ModelConfigLocal{"house-image": imageFallback},
			want:    []string{"house-image>gpt-image-2.5-flare:" + PricingRequirementImageFallback}},
		{name: "openai_native_cyber_has_provider_tariff", channelType: channeltype.OpenAI,
			models: []string{"gpt-6-cyber"}},
		{name: "azure_cyber_unpriced", channelType: channeltype.Azure,
			models: []string{"gpt-6-cyber", "gpt-4o"},
			want:   []string{"gpt-6-cyber>gpt-6-cyber:" + PricingRequirementTokenTariff}},
		{name: "azure_cyber_input_only", channelType: channeltype.Azure,
			models:  []string{"gpt-6-cyber"},
			configs: map[string]model.ModelConfigLocal{"gpt-6-cyber": {Ratio: 2}},
			want:    []string{"gpt-6-cyber>gpt-6-cyber:" + PricingRequirementTokenTariff}},
		{name: "azure_cyber_complete", channelType: channeltype.Azure,
			models:  []string{"gpt-6-cyber"},
			configs: map[string]model.ModelConfigLocal{"gpt-6-cyber": {Ratio: 2, CompletionRatio: 6}}},
		{name: "azure_alias_to_cyber", channelType: channeltype.Azure,
			models:  []string{"cyber"},
			mapping: map[string]string{"cyber": "gpt-6-cyber"},
			want:    []string{"cyber>gpt-6-cyber:" + PricingRequirementTokenTariff}},
		{name: "azure_native_only_image_unpriced_needs_both", channelType: channeltype.Azure,
			models: []string{"gpt-image-2.6-sunburst"},
			want: []string{
				"gpt-image-2.6-sunburst>gpt-image-2.6-sunburst:" + PricingRequirementImageFallback,
				"gpt-image-2.6-sunburst>gpt-image-2.6-sunburst:" + PricingRequirementTokenTariff,
			}},
		{name: "azure_native_only_image_fallback_only", channelType: channeltype.Azure,
			models:  []string{"gpt-image-2.6-sunburst"},
			configs: map[string]model.ModelConfigLocal{"gpt-image-2.6-sunburst": imageFallback},
			want:    []string{"gpt-image-2.6-sunburst>gpt-image-2.6-sunburst:" + PricingRequirementTokenTariff}},
		{name: "azure_native_only_image_complete", channelType: channeltype.Azure,
			models: []string{"gpt-image-2.6-sunburst"},
			configs: map[string]model.ModelConfigLocal{"gpt-image-2.6-sunburst": {Ratio: 2, CompletionRatio: 6,
				Image: &model.ImagePricingLocal{PricePerImageUsd: 0.04}}}},
		{name: "vertex_claude52_without_cache_prices", channelType: channeltype.VertextAI,
			models:  []string{"claude-fable-5-2"},
			configs: map[string]model.ModelConfigLocal{"claude-fable-5-2": {Ratio: 7, CompletionRatio: 3}},
			want:    []string{"claude-fable-5-2>claude-fable-5-2:" + PricingRequirementTariffSupplement}},
		{name: "vertex_claude52_complete", channelType: channeltype.VertextAI,
			models:  []string{"claude-fable-5-2"},
			configs: map[string]model.ModelConfigLocal{"claude-fable-5-2": completeOperatorTariff}},
		{name: "vertex_live_unpriced", channelType: channeltype.VertextAI,
			models: []string{"gemini-live-2.5-flash-native-audio"},
			want:   []string{"gemini-live-2.5-flash-native-audio>gemini-live-2.5-flash-native-audio:" + PricingRequirementLiveTariff}},
		{name: "vertex_live_explicitly_free", channelType: channeltype.VertextAI,
			models:  []string{"gemini-live-2.5-flash-native-audio"},
			configs: map[string]model.ModelConfigLocal{"gemini-live-2.5-flash-native-audio": {Ratio: 0}}},
		{name: "xai_legacy_slug_unpriced", channelType: channeltype.XAI,
			models: []string{"grok-beta", "grok-4"},
			want:   []string{"grok-beta>grok-beta:" + PricingRequirementTokenTariff}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := ResolveChannelPricingAdaptor(tc.channelType)
			require.NotNil(t, provider)
			gaps := EvaluateChannelPricingReadiness(provider, tc.models, tc.mapping, tc.configs, readinessAt)
			require.NotNil(t, gaps, "an all-ready channel returns an empty list, never null")
			want := tc.want
			if want == nil {
				want = []string{}
			}
			require.Equal(t, want, readinessGapKeys(gaps))
			for _, gap := range gaps {
				require.NotEmpty(t, gap.Message)
				switch gap.Requirement {
				case PricingRequirementImageFallback:
					require.Contains(t, gap.Message, "image.price_per_image_usd")
				case PricingRequirementTokenTariff:
					require.Contains(t, gap.Message, `model "`+gap.UpstreamModel+`" requires explicit positive input and output pricing in channel model_configs`)
				case PricingRequirementTariffSupplement:
					require.Contains(t, gap.Message, "cache")
				case PricingRequirementLiveTariff:
					require.Contains(t, gap.Message, "Live")
				}
			}
		})
	}
}

// megapixelImageProvider is an OpenAI adaptor whose catalog prices a strict
// GPT Image ID with a megapixel tariff, the only provider-side substitute for
// the per-image fallback that image admission accepts.
type megapixelImageProvider struct {
	openai.Adaptor
	tariff relayadaptor.ImageMegapixelPricing
}

// GetDefaultModelPricing returns the megapixel-priced catalog entry. It takes
// no parameters and returns a fresh map.
func (p *megapixelImageProvider) GetDefaultModelPricing() map[string]relayadaptor.ModelConfig {
	tariff := p.tariff
	return map[string]relayadaptor.ModelConfig{"gpt-image-2.5-flare": {Image: &relayadaptor.ImagePricingConfig{Megapixel: &tariff}}}
}

// TestStrictImageReadinessAcceptsOnlyValidMegapixelTariff proves a valid
// megapixel tariff satisfies the strict reservation as it does at admission,
// while an invalid one is reported instead of hidden.
func TestStrictImageReadinessAcceptsOnlyValidMegapixelTariff(t *testing.T) {
	valid := &megapixelImageProvider{tariff: relayadaptor.ImageMegapixelPricing{FirstMegapixelUsd: 0.02}}
	require.Empty(t, EvaluateChannelPricingReadiness(valid, []string{"gpt-image-2.5-flare"}, nil, nil, readinessAt))

	invalid := &megapixelImageProvider{tariff: relayadaptor.ImageMegapixelPricing{AdditionalMegapixelUsd: 0.01}}
	gaps := EvaluateChannelPricingReadiness(invalid, []string{"gpt-image-2.5-flare"}, nil, nil, readinessAt)
	require.Equal(t, []string{"gpt-image-2.5-flare>gpt-image-2.5-flare:" + PricingRequirementImageFallback}, readinessGapKeys(gaps))
	require.Contains(t, gaps[0].Message, "megapixel")
}

// TestResolveChannelPricingAdaptorMatchesAdmission proves the editor prices a
// draft with the adaptor admission uses, including the channel-type bind for
// OpenAI-compatible channels, and refuses unregistered types.
func TestResolveChannelPricingAdaptorMatchesAdmission(t *testing.T) {
	for _, channelType := range []int{channeltype.Unknown, -1, channeltype.Dummy, channeltype.Dummy + 1} {
		require.Nil(t, ResolveChannelPricingAdaptor(channelType), "channel type %d", channelType)
	}
	for channelType := channeltype.Unknown + 1; channelType < channeltype.Dummy; channelType++ {
		draft := ResolveChannelPricingAdaptor(channelType)
		admission := resolvePricingAdaptor(&metalib.Meta{ChannelType: channelType, APIType: channeltype.ToAPIType(channelType)})
		if admission == nil {
			require.Nil(t, draft, "channel type %d", channelType)
			continue
		}
		require.NotNil(t, draft, "channel type %d", channelType)
		require.IsType(t, admission, draft, "channel type %d", channelType)
		require.Equal(t, len(admission.GetDefaultModelPricing()), len(draft.GetDefaultModelPricing()),
			"channel type %d must be bound to the same catalog", channelType)
	}
	compatible := ResolveChannelPricingAdaptor(channeltype.Doubao)
	_, ok := compatible.(*openai.Adaptor)
	require.True(t, ok)
	require.Equal(t, channeltype.Doubao, compatible.(*openai.Adaptor).ChannelType, "an OpenAI-compatible draft must price as its own channel type")
}

// TestChannelPricingReadinessMatchesDispatchForEveryCatalog is a differential
// test across every registered channel type and every model its catalog
// suggests (plus the declared operator-only IDs): for an unpriced channel, a
// token-only tariff, and a complete tariff, the editor reports an operator or
// supplement gap exactly when the adaptor's real dispatch validator refuses
// the saved channel for missing pricing.
func TestChannelPricingReadinessMatchesDispatchForEveryCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	variants := []struct {
		name  string
		local *model.ModelConfigLocal
	}{
		{name: "unpriced"},
		{name: "token_only", local: &model.ModelConfigLocal{Ratio: 2, CompletionRatio: 6}},
		{name: "complete", local: &completeOperatorTariff},
	}
	extraModels := []string{"gpt-6-cyber", "gpt-image-2.6-sunburst", "gpt-image-2.6-flare", "claude-fable-5-2",
		"gemini-4-argon-cyber-permissive", "grok-beta", "qwen-audio-turbo", "minimaxai/minimax-m2.7"}
	checked, refusals := 0, 0
	for channelType := channeltype.Unknown + 1; channelType < channeltype.Dummy; channelType++ {
		provider := ResolveChannelPricingAdaptor(channelType)
		if provider == nil {
			continue
		}
		models := append(append([]string(nil), provider.GetModelList()...), extraModels...)
		for _, name := range models {
			if relayadaptor.IsLiveOnlyGoogleModel(name) {
				continue // Served only by a Live session; covered by the Live case test.
			}
			for _, variant := range variants {
				var configs map[string]model.ModelConfigLocal
				channel := &model.Channel{Id: 1, Type: channelType, Models: name}
				if variant.local != nil {
					configs = map[string]model.ModelConfigLocal{name: *variant.local}
					encoded, err := json.Marshal(configs)
					require.NoError(t, err)
					raw := string(encoded)
					channel.ModelConfigs = &raw
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				c.Set(ctxkey.ChannelModel, channel)
				dispatchErr := relayadaptor.ValidateRequestPricing(provider, c, &metalib.Meta{
					ChannelType: channelType, APIType: channeltype.ToAPIType(channelType),
					OriginModelName: name, ActualModelName: name, StartTime: readinessAt,
				})
				dispatchRefused := relayadaptor.IsRequestPricingNotConfigured(dispatchErr)

				operatorGap := false
				for _, gap := range EvaluateChannelPricingReadiness(provider, []string{name}, nil, configs, readinessAt) {
					if gap.Requirement == PricingRequirementTokenTariff || gap.Requirement == PricingRequirementTariffSupplement {
						operatorGap = true
					}
				}
				require.Equal(t, dispatchRefused, operatorGap, "channel type %d model %q variant %s: dispatch=%v",
					channelType, name, variant.name, dispatchErr)
				checked++
				if dispatchRefused {
					refusals++
				}
			}
		}
	}
	require.Greater(t, checked, 1000, "control: the sweep must cover the shipped catalogs")
	require.Greater(t, refusals, 10, "control: the sweep must include refused configurations")
}
