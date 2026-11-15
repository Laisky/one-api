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
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
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

// TestVertexGoogleIndependentCatalog verifies routing, independent metadata, and Developer isolation.
func TestVertexGoogleIndependentCatalog(t *testing.T) {
	for _, name := range vertexGoogleModels {
		require.NotNil(t, GetAdaptor(name))
		switch name {
		case geminiOpenaiCompatible.Gemini4CarbonModel, geminiOpenaiCompatible.Gemini4HydrogenModel:
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
	require.False(t, found)
}

// TestVertexArgonCyberExplicitPricing rejects unknown bundled prices and preserves operator data.
func TestVertexArgonCyberExplicitPricing(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	m := &meta.Meta{ActualModelName: "gemini-4-argon-cyber-permissive", OriginModelName: "alias", StartTime: time.Now()}
	require.ErrorContains(t, (&Adaptor{}).ValidateRequestPricing(c, m), "explicit")
	raw := `{"gemini-4-argon-cyber-permissive":{"ratio":7,"completion_ratio":3}}`
	ch := &dbmodel.Channel{ModelConfigs: &raw}
	c.Set(ctxkey.ChannelModel, ch)
	require.ErrorContains(t, (&Adaptor{}).ValidateRequestPricing(c, m), "cache")
	raw = `{"gemini-4-argon-cyber-permissive":{"ratio":7,"completion_ratio":3,"cached_input_ratio":0.3}}`
	require.NoError(t, (&Adaptor{}).ValidateRequestPricing(c, m))
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
