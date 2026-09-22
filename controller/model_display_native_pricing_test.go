package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestPR421NativePricingDisplay exercises the real public and authenticated
// display endpoints with persisted channel overrides. It verifies native rates,
// explicit free JSON fields, mapped/custom models and inherited metadata against
// the same resolver used by billing, rather than asserting conversion alone.
func TestPR421NativePricingDisplay(t *testing.T) {
	for _, tc := range []struct {
		name, modelName string
		channel         int
		local           *model.ModelConfigLocal
		alias, custom   bool
	}{
		{name: "default_free_per_call", modelName: "cogvideox-flash", channel: channeltype.Zhipu},
		{name: "metadata_only_per_call", modelName: "cogvideox-3", channel: channeltype.Zai, local: &model.ModelConfigLocal{MaxTokens: 123}},
		{name: "scalar_per_call", modelName: "cogvideox-3", channel: channeltype.Zai, local: &model.ModelConfigLocal{Ratio: 35}},
		{name: "scalar_audio", modelName: "voxtral-mini-tts-2603", channel: channeltype.Mistral, local: &model.ModelConfigLocal{Ratio: 2}},
		{name: "operator_paid_per_call", modelName: "cogvideox-3", channel: channeltype.Zai, local: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 20}}},
		{name: "operator_free_per_call", modelName: "cogvideox-3", channel: channeltype.Zai, local: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{}}},
		{name: "alias_per_call", modelName: "cogvideox-3", channel: channeltype.Zai, alias: true, local: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 30}}},
		{name: "override_only_per_call", modelName: "custom-call", channel: channeltype.OpenAI, custom: true, local: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 7}}},
		{name: "token_model_per_call", modelName: "gpt-4o", channel: channeltype.OpenAI, local: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 9}}},
		{name: "operator_audio", modelName: "canopylabs/orpheus-v1-english", channel: channeltype.Groq, local: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1200, InputPriceUsd: 12}}},
		{name: "operator_free_audio", modelName: "canopylabs/orpheus-v1-english", channel: channeltype.Groq, local: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1200}}},
		{name: "alias_audio", modelName: "voxtral-mini-tts-2603", channel: channeltype.Mistral, alias: true, local: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "characters", InputPriceQuantity: 1000, InputPriceUsd: 4}}},
		{name: "override_only_audio", modelName: "custom-speech", channel: channeltype.OpenAI, custom: true, local: &model.ModelConfigLocal{Audio: &model.AudioPricingLocal{InputUnit: "utf8_bytes", InputPriceQuantity: 1e6, InputPriceUsd: 3}}},
		{name: "audio_metadata_only", modelName: "voxtral-mini-tts-2603", channel: channeltype.Mistral, local: &model.ModelConfigLocal{MaxTokens: 123}},
		{name: "image_fast_path_override", modelName: "black-forest-labs/FLUX.1-schnell", channel: channeltype.SiliconFlow, local: &model.ModelConfigLocal{Image: &model.ImagePricingLocal{PricePerImageUsd: 0.45}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupModelsDisplayTestEnv(t)
			advertised := tc.modelName
			var mapping *string
			if tc.alias {
				advertised = "local-alias"
				value := fmt.Sprintf(`{"local-alias":%q}`, tc.modelName)
				mapping = &value
			}
			// Group models have an in-process TTL independent of the SQLite fixture.
			// Distinct groups prevent one case from borrowing another case's abilities.
			group := fmt.Sprintf("review-%s-%d", tc.name, time.Now().UnixNano())
			ch := &model.Channel{Name: tc.name, Type: tc.channel, Status: model.ChannelStatusEnabled, Models: advertised, Group: group, ModelMapping: mapping}
			if tc.custom {
				ch.Models = ""
			}
			local := map[string]model.ModelConfigLocal{}
			if tc.local != nil {
				local[tc.modelName] = *tc.local
				require.NoError(t, ch.SetModelPriceConfigs(local))
			}
			require.NoError(t, model.DB.Create(ch).Error)
			user := &model.User{Username: "review-display", Password: "fixture", Group: group, Status: model.UserStatusEnabled}
			require.NoError(t, model.DB.Create(user).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: user.Group, Model: advertised, ChannelId: ch.Id, Enabled: true}).Error)
			provider := relay.GetAdaptor(channeltype.ToAPIType(tc.channel))
			provider.Init(&metalib.Meta{ChannelType: tc.channel})
			for _, loggedIn := range []bool{false, true} {
				// Config-only entries are documented as anonymous discovery;
				// authenticated listings still obey the channel's Models list.
				if tc.custom && loggedIn {
					continue
				}
				t.Run(fmt.Sprintf("authenticated_%v", loggedIn), func(t *testing.T) {
					router := gin.New()
					router.GET("/api/models/display", func(c *gin.Context) {
						if loggedIn {
							c.Set(ctxkey.Id, user.Id)
						}
						GetModelsDisplay(c)
					})
					w := httptest.NewRecorder()
					router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/models/display", nil))
					require.Equal(t, http.StatusOK, w.Code)
					var response ModelsDisplayResponse
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
					require.True(t, response.Success, response.Message)
					key := channeltype.IdToName(tc.channel) + ":" + ch.Name
					require.Contains(t, response.Data, key)
					require.Contains(t, response.Data[key].Models, advertised)
					got := response.Data[key].Models[advertised]
					resolved, ok := pricing.ResolveModelConfig(tc.modelName, local, provider, time.Now())
					require.True(t, ok)
					providerCfg, _ := pricing.ResolveModelConfig(tc.modelName, nil, provider, time.Now())
					if resolved.PerCall == nil && providerCfg.PerCall != nil && resolved.Video == nil {
						resolved.PerCall = providerCfg.PerCall.Clone()
						if tc.name == "scalar_per_call" {
							resolved.PerCall.UsdPerThousandCalls = 0.07 // 35 quota/call at 500K quota/USD.
						}
					}
					if resolved.PerCall != nil {
						require.Equal(t, buildPerCallDisplayPricing(resolved.PerCall), got.PerCallPricing)
						require.Zero(t, got.InputPrice)
						require.Zero(t, got.OutputPrice)
						require.Zero(t, got.CachedInputPrice)
						require.Zero(t, got.CacheWrite5mPrice)
						require.Zero(t, got.CacheWrite1hPrice)
						require.Empty(t, got.Tiers)
						encoded, err := json.Marshal(got.PerCallPricing)
						require.NoError(t, err)
						var fields map[string]json.RawMessage
						require.NoError(t, json.Unmarshal(encoded, &fields))
						require.Contains(t, fields, "usd_per_thousand_calls")
						require.Contains(t, fields, "usd_per_call")
					}
					audio, hasAudio := pricing.ResolveAudioPricing(tc.modelName, local, provider, time.Now())
					if hasAudio {
						if tc.name == "scalar_audio" {
							require.NotNil(t, got.AudioPricing)
							require.Equal(t, "characters", got.AudioPricing.InputUnit)
							require.Equal(t, 1000000.0, got.AudioPricing.InputPriceQuantity)
							require.Equal(t, 4.0, got.AudioPricing.InputPriceUsd) // 2 quota/character.
						} else {
							require.Equal(t, buildAudioDisplayPricing(audio), got.AudioPricing)
						}
					}
					if tc.local != nil && tc.local.Image != nil {
						require.InDelta(t, tc.local.Image.PricePerImageUsd, got.ImagePrice, 1e-12)
						require.NotNil(t, got.ImagePricing)
						require.InDelta(t, tc.local.Image.PricePerImageUsd, got.ImagePricing.PricePerImageUsd, 1e-12)
					}
				})
			}
		})
	}
}

// TestPR421FreePerCallWindowJSON verifies that zero survives the actual overlay
// response type, so a free dated tariff can be distinguished from no override.
func TestPR421FreePerCallWindowJSON(t *testing.T) {
	result := buildTimeWindowOverlayDisplayWithBase(adaptorpkg.ModelConfig{PerCall: &adaptorpkg.PerCallPricingConfig{}}, 0, 0, func(v float64) float64 { return v })
	body, err := json.Marshal(result)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fields))
	require.JSONEq(t, `{"usd_per_thousand_calls":0,"usd_per_call":0}`, string(fields["per_call_pricing"]))
}
