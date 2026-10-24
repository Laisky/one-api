package moonshot_test

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/moonshot"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// TestMoonshotCurrentCatalogPricingAndLimits checks the public catalog and price
// resolver against the official October 24 CNY tariff and model limits using t.
func TestMoonshotCurrentCatalogPricingAndLimits(t *testing.T) {
	a := &moonshot.Adaptor{}
	for _, tc := range []struct {
		name                           string
		input, output, hit, five, hour float64
		context                        int32
	}{
		{"kimi-k3.1-pro", 13, 68, 1.3, 13, 26, 1048576},
		{"kimi-k3.1-pro-highspeed", 26, 136, 1.3, 26, 52, 1048576},
		{"kimi-k3", 20, 100, 2, 20, 40, 1048576},
		{"kimi-k2.7-code", 6.5, 27, 1.3, 0, 0, 262144},
		{"kimi-k2.7-code-highspeed", 13, 54, 2.6, 0, 0, 262144},
		{"kimi-k2.6", 6.5, 27, 1.1, 0, 0, 262144},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Contains(t, a.GetModelList(), tc.name)
			cfg, ok := pricing.ResolveModelConfig(tc.name, nil, a, time.Time{})
			require.True(t, ok)
			require.InDelta(t, tc.input, cfg.Ratio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, tc.output, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, tc.hit, cfg.CachedInputRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, tc.five, cfg.CacheWrite5mRatio/ratio.MilliTokensRmb, 1e-12)
			require.InDelta(t, tc.hour, cfg.CacheWrite1hRatio/ratio.MilliTokensRmb, 1e-12)
			require.Equal(t, tc.context, cfg.ContextLength)
			require.Equal(t, tc.context, cfg.MaxOutputTokens, "completion budget is limited by remaining context, not the default")
			require.Zero(t, cfg.MaxReasoningTokens, "no independent reasoning-token ceiling is published")
			require.ElementsMatch(t, []string{"text", "image", "video"}, cfg.InputModalities)
			require.NotContains(t, cfg.SupportedFeatures, "web_search", "upstream warns against production search use")
			require.Equal(t, cfg, openai.GetCompatibleChannelPricing(channeltype.Moonshot)[tc.name])
			if tc.context == 262144 {
				require.Contains(t, cfg.Description, "2026-10-31")
				require.Contains(t, cfg.Description, "December 2026")
				require.NotContains(t, cfg.Description, "Retired on")
			}
		})
	}
}

// TestMoonshotCurrentModelsWireCompatibility verifies fixed sampling is omitted
// on Chat, Messages, and Responses conversion paths without rewriting model IDs
// or a caller's output budget above the K2 default. It reports failures through t.
func TestMoonshotCurrentModelsWireCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := &moonshot.Adaptor{}
	for _, name := range []string{"kimi-k3.1-pro", "kimi-k3.1-pro-highspeed", "kimi-k3", "kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.6"} {
		for _, format := range []string{"chat", "messages", "responses"} {
			t.Run(name+"/"+format, func(t *testing.T) {
				temperature, topP, penalty, n, limit := 0.7, 0.5, 1.0, 2, 65536
				var converted any
				var err error
				switch format {
				case "chat":
					converted, err = a.ConvertRequest(nil, 0, &model.GeneralOpenAIRequest{
						Model: name, Messages: []model.Message{{Role: "user", Content: "hello"}},
						Temperature: &temperature, TopP: &topP, N: &n,
						PresencePenalty: &penalty, FrequencyPenalty: &penalty,
						MaxCompletionTokens: &limit,
					})
				case "messages":
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					converted, err = a.ConvertClaudeRequest(c, &model.ClaudeRequest{
						Model: name, Messages: []model.ClaudeMessage{{Role: "user", Content: "hello"}},
						Temperature: &temperature, TopP: &topP, MaxTokens: limit,
					})
				case "responses":
					request, conversionErr := openai.ConvertResponseAPIToChatCompletionRequest(&openai.ResponseAPIRequest{
						Model: name, Input: openai.ResponseAPIInput{"hello"},
						Temperature: &temperature, TopP: &topP, MaxOutputTokens: &limit,
					})
					require.NoError(t, conversionErr)
					converted, err = a.ConvertRequest(nil, 0, request)
				}
				require.NoError(t, err)
				encoded, err := json.Marshal(converted)
				require.NoError(t, err)
				var wire map[string]any
				require.NoError(t, json.Unmarshal(encoded, &wire))
				require.Equal(t, name, wire["model"])
				require.Equal(t, float64(limit), wire["max_completion_tokens"])
				require.NotEmpty(t, wire["messages"])
				for _, field := range []string{"temperature", "top_p", "n", "presence_penalty", "frequency_penalty"} {
					require.NotContains(t, wire, field)
				}
			})
		}
	}
}

// TestMoonshotK31ReasoningEffortPreserved verifies native and OpenAI effort
// spellings survive conversion with the documented K3 tiers using t.
func TestMoonshotK31ReasoningEffortPreserved(t *testing.T) {
	a := &moonshot.Adaptor{}
	for _, name := range []string{"kimi-k3.1-pro", "kimi-k3.1-pro-highspeed"} {
		for input, expected := range map[string]string{"low": "low", "high": "high", "max": "max", "medium": "high", "xhigh": "max"} {
			t.Run(name+"/"+input, func(t *testing.T) {
				converted, err := a.ConvertRequest(nil, 0, &model.GeneralOpenAIRequest{Model: name, ReasoningEffort: &input})
				require.NoError(t, err)
				request, ok := converted.(*model.GeneralOpenAIRequest)
				require.True(t, ok)
				require.NotNil(t, request.ReasoningEffort)
				require.Equal(t, expected, *request.ReasoningEffort)
				require.Equal(t, name, request.Model)
			})
		}
	}
}

// TestMoonshotK31QuotaAndOverrides exercises provider defaults and explicit
// operator tariffs with mutually exclusive input buckets using t. Canonical
// write counts are supplied here; this does not qualify native Kimi receipts.
func TestMoonshotK31QuotaAndOverrides(t *testing.T) {
	a := &moonshot.Adaptor{}
	for _, tc := range []struct {
		name                           string
		input, output, hit, five, hour float64
	}{
		{"kimi-k3.1-pro", 13, 68, 1.3, 13, 26},
		{"kimi-k3.1-pro-highspeed", 26, 136, 1.3, 26, 52},
		{"kimi-k3", 20, 100, 2, 20, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := quota.ComputeInput{
				ModelName: tc.name, GroupRatio: 1, PricingAdaptor: a,
				Usage: &model.Usage{
					PromptTokens: 10000, CompletionTokens: 500,
					PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: 2000},
					CacheWrite5mTokens:  3000, CacheWrite1hTokens: 4000,
				},
			}
			result := quota.Compute(input)
			expected := (1000*tc.input + 500*tc.output + 2000*tc.hit + 3000*tc.five + 4000*tc.hour) * ratio.MilliTokensRmb
			require.Equal(t, int64(math.Ceil(expected)), result.TotalQuota)
			input.ChannelModelConfigs = map[string]modelcfg.ModelConfigLocal{
				tc.name: {Ratio: 2, CompletionRatio: 3, CachedInputRatio: 0.5, CacheWrite5mRatio: 4, CacheWrite1hRatio: 6},
			}
			result = quota.Compute(input)
			require.EqualValues(t, 1000*2+500*6+2000*0.5+3000*4+4000*6, result.TotalQuota)
		})
	}
}
