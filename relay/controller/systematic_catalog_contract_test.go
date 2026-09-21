package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	persistmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestSystematicCatalogReasoningQuery checks every advertised effort on every
// registered channel through both query-injection paths. It takes a test handle
// and reports contract violations without making network requests.
func TestSystematicCatalogReasoningQuery(t *testing.T) {
	channels, models, efforts := 0, 0, 0
	for channel := channeltype.Unknown + 1; channel < channeltype.Dummy; channel++ {
		meta := &metalib.Meta{APIType: channeltype.ToAPIType(channel), ChannelType: channel}
		provider := resolvePricingAdaptor(meta)
		require.NotNil(t, provider, "channel %d", channel)
		channels++
		if meta.APIType == apitype.Anthropic || meta.APIType == apitype.AwsClaude {
			continue
		}
		for name, cfg := range provider.GetDefaultModelPricing() {
			if len(cfg.SupportedReasoningEfforts) == 0 {
				continue
			}
			models++
			for _, effort := range cfg.SupportedReasoningEfforts {
				efforts++
				for _, response := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/responses=%t", channeltype.IdToName(channel), name, effort, response), func(t *testing.T) {
						ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
						ctx.Request = httptest.NewRequest("POST", "/?thinking=true&reasoning_effort="+url.QueryEscape(effort), nil)
						mapped := *meta
						mapped.ActualModelName = name
						if response {
							req := &openai.ResponseAPIRequest{Model: "public-alias"}
							applyThinkingQueryToResponseRequest(ctx, req, &mapped)
							require.NotNil(t, req.Reasoning)
							require.NotNil(t, req.Reasoning.Effort)
							require.Equal(t, effort, *req.Reasoning.Effort)
						} else {
							req := &relaymodel.GeneralOpenAIRequest{Model: "public-alias"}
							applyThinkingQueryToChatRequest(ctx, req, &mapped)
							require.NotNil(t, req.ReasoningEffort)
							require.Equal(t, effort, *req.ReasoningEffort)
						}
					})
				}
			}
		}
	}
	t.Logf("catalog sweep: channels=%d model-channel-pairs=%d advertised-efforts=%d API-cases=%d", channels, models, efforts, efforts*2)
}

// TestSystematicPricingAdaptorFallback checks that channel IDs are translated
// rather than mistaken for API IDs. It takes a test handle and reports failures.
func TestSystematicPricingAdaptorFallback(t *testing.T) {
	for channel := channeltype.Unknown + 1; channel < channeltype.Dummy; channel++ {
		t.Run(channeltype.IdToName(channel), func(t *testing.T) {
			expected := resolvePricingAdaptor(&metalib.Meta{APIType: channeltype.ToAPIType(channel), ChannelType: channel})
			actual := resolvePricingAdaptor(&metalib.Meta{APIType: -1, ChannelType: channel})
			require.NotNil(t, actual)
			require.Equal(t, fmt.Sprintf("%T", expected), fmt.Sprintf("%T", actual))
			require.Equal(t, expected.GetDefaultModelPricing(), actual.GetDefaultModelPricing())
		})
	}
	require.Nil(t, resolvePricingAdaptor(nil))
	require.Nil(t, resolvePricingAdaptor(&metalib.Meta{APIType: -1, ChannelType: -1}))
}

// TestSystematicResponseReasoningWire verifies that normalized known fields reach
// upstream while unknown nested provider fields survive. It takes a test handle
// and returns no value; failures describe the wire contract that was violated.
func TestSystematicResponseReasoningWire(t *testing.T) {
	for _, channel := range []int{channeltype.OpenAI, channeltype.Azure, channeltype.XAI, channeltype.OpenAICompatible} {
		for _, tc := range []struct {
			name, raw string
			effort    *string
		}{
			{"insert", `{"model":"grok-4.7","input":"hello","vendor_root":{"counter":9007199254740993}}`, stringPtr("xhigh")},
			{"replace", `{"model":"grok-4.7","input":"hello","reasoning":{"effort":"low","vendor_option":{"counter":9007199254740993}}}`, stringPtr("high")},
			{"delete_known", `{"model":"grok-4.7","input":"hello","reasoning":{"effort":"none","vendor_option":{"counter":9007199254740993}}}`, nil},
		} {
			t.Run(fmt.Sprintf("%s/%s", channeltype.IdToName(channel), tc.name), func(t *testing.T) {
				var req openai.ResponseAPIRequest
				require.NoError(t, json.Unmarshal([]byte(tc.raw), &req))
				req.Reasoning = &relaymodel.OpenAIResponseReasoning{Effort: tc.effort}
				body, _, _, err := normalizeResponseAPIRawBody([]byte(tc.raw), &req, channel)
				require.NoError(t, err)
				var root map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body, &root))
				var reasoning map[string]json.RawMessage
				if raw := root["reasoning"]; len(raw) > 0 {
					require.NoError(t, json.Unmarshal(raw, &reasoning))
				}
				if tc.effort == nil {
					require.NotContains(t, reasoning, "effort")
				} else {
					require.JSONEq(t, fmt.Sprintf("%q", *tc.effort), string(reasoning["effort"]))
				}
				if tc.name == "insert" {
					require.Contains(t, string(root["vendor_root"]), "9007199254740993")
				} else {
					require.Contains(t, string(reasoning["vendor_option"]), "9007199254740993")
				}
				again, _, changed, err := normalizeResponseAPIRawBody(body, &req, channel)
				require.NoError(t, err)
				require.False(t, changed, "normalization must be idempotent")
				require.Equal(t, body, again)
			})
		}
	}
}

// TestSystematicImageTierWire verifies that accepted normalized image tiers and
// the serialized upstream request agree. It takes a test handle and uses no API.
func TestSystematicImageTierWire(t *testing.T) {
	for _, size := range []string{"2048x2048", "2048X2048", "2048 × 2048", "2048*2048"} {
		for _, quality := range []string{"medium", " Medium "} {
			t.Run(size+"/"+quality, func(t *testing.T) {
				req := &relaymodel.ImageRequest{Model: "grok-imagine-image-2.0", Prompt: "test", N: 1, Size: size, Quality: quality}
				cfg := xai.ModelRatios[req.Model].Image.Clone()
				applyImageDefaults(req, cfg)
				require.Nil(t, validateImageRequest(req, nil, cfg))
				multiplier, err := getImageCostRatio(req, cfg)
				require.NoError(t, err)
				require.Equal(t, 2.0, multiplier)
				converted, err := (&xai.Adaptor{}).ConvertImageRequest(nil, req)
				require.NoError(t, err)
				wire, err := json.Marshal(converted)
				require.NoError(t, err)
				var root map[string]any
				require.NoError(t, json.Unmarshal(wire, &root))
				require.Equal(t, "2k", root["resolution"])
				require.Equal(t, "medium", root["quality"])
				require.NotContains(t, root, "size")
			})
		}
	}
}

// TestSystematicImageRequestDefaultsOverride verifies that a defaults-only
// channel override does not discard the provider tariff. It takes a test handle
// and returns no value; no real inference or billable resource is used.
func TestSystematicImageRequestDefaultsOverride(t *testing.T) {
	const name = "grok-imagine-image-2.0"
	for _, tc := range []struct {
		name          string
		local         *persistmodel.ImagePricingLocal
		size, quality string
	}{
		{"quality", &persistmodel.ImagePricingLocal{DefaultQuality: "medium"}, "1024x1024", "medium"},
		{"size", &persistmodel.ImagePricingLocal{DefaultSize: "2048x2048"}, "2048x2048", "auto"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, ok := pricing.ResolveImagePricing(name, map[string]persistmodel.ModelConfigLocal{name: {Image: tc.local}}, &xai.Adaptor{}, time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC))
			require.True(t, ok)
			require.NotNil(t, cfg)
			req := &relaymodel.ImageRequest{Model: name, Prompt: "test", N: 1}
			applyImageDefaults(req, cfg)
			require.Equal(t, tc.size, req.Size)
			require.Equal(t, tc.quality, req.Quality)
			require.InDelta(t, 0.04, cfg.PricePerImageUsd, 1e-12)
			require.Nil(t, validateImageRequest(req, nil, cfg))
		})
	}
}

// TestSystematicCatalogModelExposure verifies that priced models are discoverable
// and that declared defaults belong to the advertised effort vocabulary. It takes
// a test handle and reports discrepancies across all registered channel types.
func TestSystematicCatalogModelExposure(t *testing.T) {
	for channel := channeltype.Unknown + 1; channel < channeltype.Dummy; channel++ {
		p := resolvePricingAdaptor(&metalib.Meta{APIType: channeltype.ToAPIType(channel), ChannelType: channel})
		require.NotNil(t, p)
		listed := p.GetModelList()
		for name, cfg := range p.GetDefaultModelPricing() {
			t.Run(fmt.Sprintf("%s/%s", channeltype.IdToName(channel), name), func(t *testing.T) {
				require.NotEmpty(t, strings.TrimSpace(name))
				require.True(t, slices.Contains(listed, name), "priced model is missing from model list")
				if cfg.DefaultReasoningEffort != "" {
					require.Contains(t, cfg.SupportedReasoningEfforts, cfg.DefaultReasoningEffort)
				}
			})
		}
	}
}
