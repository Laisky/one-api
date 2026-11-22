package vertexai

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	nativegemini "github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexGoogleNativeControls exercises actual mapped conversion without network calls.
func TestVertexGoogleNativeControls(t *testing.T) {
	for _, tc := range []struct {
		name, effort, want string
		stream             bool
		tokens             int
		invalid            bool
	}{
		{"gemini-4-neon", "", "HIGH", false, 32768, false},
		{"gemini-4-neon", "xhigh", "XHIGH", true, 32768, false},
		{"gemini-4-neon", "minimal", "", false, 100, true},
		{"gemini-4-neon", "none", "", false, 100, true},
		{"gemini-4-neon", "high", "", false, 32769, true},
		{"gemini-4-neon", "xhigh", "", true, 32769, true},
		{"gemini-4-hydrogen", "", "HIGH", false, 65536, false},
		{"gemini-4-hydrogen", "xhigh", "XHIGH", true, 65536, false},
		{"gemini-4-hydrogen", "minimal", "", false, 100, true},
		{"gemini-4-hydrogen", "none", "", false, 100, true},
		{"gemini-4-hydrogen", "max", "", false, 100, true},
		{"gemini-4-hydrogen", "high", "", false, 65537, true},
		{"gemini-4-carbon", "", "MEDIUM", false, 32768, false},
		{"gemini-4-carbon", "low", "LOW", true, 65536, false},
		{"gemini-4-carbon", "high", "", false, 32769, true},
		{"gemini-4-carbon", "xhigh", "", false, 100, true},
		{"gemini-4-carbon-cyber-permissive", "", "HIGH", false, 100, false},
		{"gemini-4-argon-cyber-permissive", "", "HIGH", false, 100, false},
		{"gemini-4-argon-cyber-permissive", "high", "HIGH", true, 32768, false},
		{"gemini-4-argon-cyber-permissive", "high", "", true, 32769, true},
		{"gemini-3.8-flash-cyber", "medium", "MEDIUM", false, 65536, false},
	} {
		t.Run(tc.name+tc.effort+tc.want+strconv.Itoa(tc.tokens), func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: tc.name})
			temp := .4
			req := &model.GeneralOpenAIRequest{Model: "customer-alias", Messages: []model.Message{{Role: "user", Content: "Hello"}}, MaxTokens: tc.tokens, Stream: tc.stream, Temperature: &temp}
			if tc.effort != "" {
				req.ReasoningEffort = &tc.effort
			}
			before, err := json.Marshal(req)
			require.NoError(t, err)
			result, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
			if tc.invalid {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				native := result.(*nativegemini.ChatRequest)
				require.NotNil(t, native.GenerationConfig.ThinkingConfig)
				require.Equal(t, tc.want, native.GenerationConfig.ThinkingConfig.ThinkingLevel)
				require.Equal(t, tc.tokens, native.GenerationConfig.MaxOutputTokens)
				if tc.name == "gemini-3.8-flash-cyber" {
					require.Equal(t, &temp, native.GenerationConfig.Temperature)
				} else {
					require.Nil(t, native.GenerationConfig.Temperature)
				}
			}
			after, err := json.Marshal(req)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

// TestVertexGoogleRejectBudgetAndHonorCompletionLimit covers alternate ingress controls.
func TestVertexGoogleRejectBudgetAndHonorCompletionLimit(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "gemini-4-hydrogen"})
	budget := 2048
	req := &model.GeneralOpenAIRequest{Model: "gemini-4-hydrogen", Messages: []model.Message{{Role: "user", Content: "Hi"}}, Thinking: &model.Thinking{Type: "enabled", BudgetTokens: &budget}}
	_, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.ErrorContains(t, err, "thinking")
	req.Thinking = nil
	limit := 65537
	req.MaxCompletionTokens = &limit
	_, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.ErrorContains(t, err, "foreground")
	req.MaxCompletionTokens = nil
	effort := "xhigh"
	req.Reasoning = &model.OpenAIResponseReasoning{Effort: &effort}
	result, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	require.Equal(t, "XHIGH", result.(*nativegemini.ChatRequest).GenerationConfig.ThinkingConfig.ThinkingLevel)
	_, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, nil)
	require.ErrorContains(t, err, "nil")
}

// TestVertexNewTariffForegroundBoundaries prevents omitted and alternate token
// controls from bypassing the qualified 32K paid-usage boundary. Parameter t asserts.
func TestVertexNewTariffForegroundBoundaries(t *testing.T) {
	for _, name := range []string{"gemini-4-neon", argonCyberModel} {
		for _, stream := range []bool{false, true} {
			converted := &nativegemini.ChatRequest{}
			require.NoError(t, configureVertexGoogleRequest(name, &model.GeneralOpenAIRequest{Stream: stream}, converted))
			require.Equal(t, 32768, converted.GenerationConfig.MaxOutputTokens)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: name})
			limit := 32769
			request := &model.GeneralOpenAIRequest{Model: "alias", Stream: stream, MaxCompletionTokens: &limit,
				Messages: []model.Message{{Role: "user", Content: "Hello"}}}
			_, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
			require.ErrorContains(t, err, "foreground output limit is 32768")
			require.Equal(t, 32769, *request.MaxCompletionTokens)
		}
	}
}

// TestVertexGoogleIndependentCatalog verifies routing, independent metadata, and Developer isolation.
func TestVertexGoogleIndependentCatalog(t *testing.T) {
	for _, name := range vertexGoogleModels {
		require.NotNil(t, GetAdaptor(name))
		switch name {
		case geminiOpenaiCompatible.Gemini4CarbonModel, geminiOpenaiCompatible.Gemini4HydrogenModel, geminiOpenaiCompatible.Gemini4NeonModel:
			require.Contains(t, geminiOpenaiCompatible.ModelRatios, name, "published Developer IDs remain shared")
		case "gemini-3.8-flash-cyber":
			// Preserve the existing exception: Developer discovery is managed independently.
		default:
			require.NotContains(t, geminiOpenaiCompatible.ModelRatios, name, "Vertex-only variants must not leak into Developer discovery")
		}
		m := &meta.Meta{ActualModelName: name, Mode: relaymode.ChatCompletions}
		m.Config.VertexAIProjectID = "offline-project"
		url, err := (&Adaptor{}).GetRequestURL(m)
		require.NoError(t, err)
		require.Contains(t, url, "/v1/projects/offline-project/locations/global/publishers/google/models/"+name+":generateContent")
	}
	a := VertexGoogleModelConfigs()
	a["gemini-4-hydrogen"].SupportedReasoningEfforts[0] = "changed"
	require.Equal(t, "low", VertexGoogleModelConfigs()["gemini-4-hydrogen"].SupportedReasoningEfforts[0])
	require.Equal(t, "low", geminiOpenaiCompatible.ModelRatios[geminiOpenaiCompatible.Gemini4HydrogenModel].SupportedReasoningEfforts[0], "Vertex metadata mutations must not affect the Developer catalogue")
	_, found := (&Adaptor{}).GetDefaultModelPricing()["gemini-4-argon-cyber-permissive"]
	require.True(t, found, "Cloud now publishes an independent Argon Cyber Standard tariff")
}

// TestVertexArgonCyberProviderPricing verifies the published host tariff takes
// effect without deleting or overriding a channel's existing prices. Parameter t asserts.
func TestVertexArgonCyberProviderPricing(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	m := &meta.Meta{ActualModelName: argonCyberModel, OriginModelName: "alias", StartTime: time.Now()}
	a := &Adaptor{}
	require.False(t, a.RequiresOperatorTariff(argonCyberModel))
	require.True(t, a.RequiresOperatorTariff("claude-fable-5-2"))
	require.True(t, a.RequiresOperatorTariff("gemini-3.8-live"))
	require.NoError(t, a.ValidateRequestPricing(c, m))
	require.NoError(t, a.ValidateOperatorTariffSupplement(argonCyberModel, nil, m.StartTime))
	base, found := pricing.ResolveModelConfig(argonCyberModel, nil, a, m.StartTime)
	require.True(t, found)
	require.InDelta(t, 4, base.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 20, base.Ratio*base.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, .20, base.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
	require.EqualValues(t, 32768, base.MaxOutputTokens)
	require.Empty(t, base.TimeWindows, "Cyber variants receive no promotional credits")
	raw := `{"gemini-4-argon-cyber-permissive":{"ratio":7,"completion_ratio":3,"cached_input_ratio":0.3}}`
	ch := &dbmodel.Channel{ModelConfigs: &raw}
	c.Set(ctxkey.ChannelModel, ch)
	require.NoError(t, a.ValidateRequestPricing(c, m))
	custom, found := pricing.ResolveModelConfig(argonCyberModel, ch.GetModelPriceConfigs(), a, m.StartTime)
	require.True(t, found)
	require.Equal(t, 7.0, custom.Ratio)
	require.Equal(t, 3.0, custom.CompletionRatio)
	require.Equal(t, .3, custom.CachedInputRatio)
	require.Equal(t, raw, *ch.ModelConfigs)
}

// TestVertexGoogleEffortConflict rejects ambiguous ingress while accepting matching carriers.
func TestVertexGoogleEffortConflict(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "gemini-4-hydrogen"})
	high, low := "high", "low"
	req := &model.GeneralOpenAIRequest{Model: "alias", Messages: []model.Message{{Role: "user", Content: "Hello"}}, ReasoningEffort: &high, Reasoning: &model.OpenAIResponseReasoning{Effort: &low}}
	_, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.ErrorContains(t, err, "conflicting")
	req.Reasoning.Effort = &high
	result, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	require.Equal(t, "HIGH", result.(*nativegemini.ChatRequest).GenerationConfig.ThinkingConfig.ThinkingLevel)
}

// TestVertexGoogleExistingModelControl proves ordinary and legacy model controls remain unchanged.
func TestVertexGoogleExistingModelControl(t *testing.T) {
	for _, name := range []string{"gemini-4-argon", "gemini-2.5-flash", "claude-sonnet-4-5"} {
		temp := .7
		converted := &nativegemini.ChatRequest{GenerationConfig: nativegemini.ChatGenerationConfig{Temperature: &temp, MaxOutputTokens: 100000}}
		require.NoError(t, configureVertexGoogleRequest(name, &model.GeneralOpenAIRequest{}, converted))
		require.Same(t, &temp, converted.GenerationConfig.Temperature)
		require.Nil(t, converted.GenerationConfig.ThinkingConfig)
		require.Equal(t, 100000, converted.GenerationConfig.MaxOutputTokens)
	}
}

// TestVertexFlashCyberSamplingPreserved prevents Gemini 4 normalization from changing Flash requests.
func TestVertexFlashCyberSamplingPreserved(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "gemini-3.8-flash-cyber"})
	temp, topP := .4, .8
	req := &model.GeneralOpenAIRequest{Model: "alias", Messages: []model.Message{{Role: "user", Content: "Hello"}}, Temperature: &temp, TopP: &topP}
	before, err := json.Marshal(req)
	require.NoError(t, err)
	result, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	native := result.(*nativegemini.ChatRequest)
	require.Equal(t, &temp, native.GenerationConfig.Temperature)
	require.Equal(t, &topP, native.GenerationConfig.TopP)
	after, err := json.Marshal(req)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	cfg := VertexGoogleModelConfigs()["gemini-3.8-flash-cyber"]
	require.Contains(t, cfg.SupportedSamplingParameters, "temperature")
	require.Contains(t, cfg.SupportedSamplingParameters, "top_p")
}
