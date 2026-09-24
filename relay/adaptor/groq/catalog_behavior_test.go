package groq

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
)

// TestGroqCatalogCapabilities checks the published capability matrix using t and returns no value.
func TestGroqCatalogCapabilities(t *testing.T) {
	t.Parallel()

	var structuredModels, cachedModels []string
	for _, modelID := range (&Adaptor{}).GetModelList() {
		config, ok := (&Adaptor{}).GetDefaultModelPricing()[modelID]
		require.True(t, ok, modelID)
		for _, unsupported := range []string{
			"frequency_penalty", "presence_penalty", "logprobs", "top_logprobs", "logit_bias", "top_k", "min_p",
		} {
			require.NotContains(t, config.SupportedSamplingParameters, unsupported, modelID)
		}
		for _, feature := range config.SupportedFeatures {
			if feature == "structured_outputs" {
				structuredModels = append(structuredModels, modelID)
			}
		}
		if config.CachedInputRatio != 0 {
			cachedModels = append(cachedModels, modelID)
			require.InDelta(t, config.Ratio/2, config.CachedInputRatio, 1e-12, modelID)
		}
		if len(config.SupportedReasoningEfforts) > 0 {
			require.Contains(t, config.SupportedReasoningEfforts, config.DefaultReasoningEffort, modelID)
			require.Contains(t, config.SupportedSamplingParameters, "reasoning_effort", modelID)
			require.Contains(t, config.SupportedSamplingParameters, "temperature", modelID)
			require.Contains(t, config.SupportedSamplingParameters, "top_p", modelID)
			require.Contains(t, config.SupportedSamplingParameters, "max_completion_tokens", modelID)
		}
	}

	// Sources: Groq's structured-outputs and prompt-caching guides, audited 2026-09-24.
	require.ElementsMatch(t, []string{
		"openai/gpt-oss-120b", "openai/gpt-oss-20b", "openai/gpt-oss-safeguard-20b", "qwen/qwen3.8-27b",
	}, structuredModels)
	require.ElementsMatch(t, []string{
		"openai/gpt-oss-120b", "openai/gpt-oss-20b", "openai/gpt-oss-safeguard-20b",
	}, cachedModels)
}

// TestGroqLegacyPricingCompatibility checks retained historical rates using t and returns no value.
func TestGroqLegacyPricingCompatibility(t *testing.T) {
	t.Parallel()

	// These are compatibility defaults, not current enterprise quotes. Updating
	// discovery must not silently make an existing paid configuration free.
	for modelID, prices := range map[string][2]float64{
		"llama-3.1-8b-instant":                     {0.05, 0.08},
		"llama-3.3-70b-versatile":                  {0.59, 0.79},
		"meta-llama/llama-4-scout-17b-16e-instruct": {0.11, 0.34},
		"qwen/qwen3-32b":                          {0.29, 0.59},
		"qwen/qwen3.6-27b":                        {0.60, 3.00},
	} {
		t.Run(modelID, func(t *testing.T) {
			t.Parallel()
			a := &Adaptor{}
			require.Contains(t, a.GetDefaultModelPricing(), modelID)
			input := a.GetModelRatio(modelID) / ratio.MilliTokensUsd
			require.InDelta(t, prices[0], input, 1e-12)
			require.InDelta(t, prices[1], input*a.GetCompletionRatio(modelID), 1e-12)
		})
	}
}

// TestGroqModelListIsIndependent checks that callers cannot mutate discovery using t and returns no value.
func TestGroqModelListIsIndependent(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	first, second := a.GetModelList(), a.GetModelList()
	require.NotEmpty(t, first)
	require.Equal(t, first, second)
	first[0] = "caller-local-model"
	require.NotContains(t, second, "caller-local-model")
	require.Equal(t, second, a.GetModelList())
}

// TestGroqQwen38ReasoningConversion checks chat and Responses-style effort forwarding using t and returns no value.
func TestGroqQwen38ReasoningConversion(t *testing.T) {
	t.Parallel()

	for _, effort := range []string{"none", "default", "low", "medium", "high"} {
		for _, source := range []string{"chat", "responses"} {
			t.Run(source+"/"+effort, func(t *testing.T) {
				t.Parallel()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				req := &model.GeneralOpenAIRequest{Model: "qwen/qwen3.8-27b"}
				if source == "chat" {
					req.ReasoningEffort = &effort
				} else {
					req.Reasoning = &model.OpenAIResponseReasoning{Effort: &effort}
				}

				converted, err := (&Adaptor{}).ConvertRequest(c, 0, req)
				require.NoError(t, err)
				encoded, err := json.Marshal(converted)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(encoded, &payload))
				require.Equal(t, "qwen/qwen3.8-27b", payload["model"])
				require.Equal(t, effort, payload["reasoning_effort"])
				require.NotContains(t, payload, "reasoning")
			})
		}
	}
}

// TestGroqQwen38ReasoningBoundaries checks effort precedence and invalid values using t and returns no value.
func TestGroqQwen38ReasoningBoundaries(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		direct     *string
		promoted   *string
		wantEffort *string
	}{
		{name: "omitted"},
		{name: "explicit chat wins", direct: strPtr("none"), promoted: strPtr("high"), wantEffort: strPtr("none")},
		{name: "empty is omitted", direct: strPtr("")},
		{name: "minimal is not supported", direct: strPtr("minimal")},
		{name: "native xhigh is not an API value", promoted: strPtr("xhigh")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := &model.GeneralOpenAIRequest{Model: "qwen/qwen3.8-27b", ReasoningEffort: tc.direct}
			if tc.promoted != nil {
				req.Reasoning = &model.OpenAIResponseReasoning{Effort: tc.promoted}
			}
			converted, err := (&Adaptor{}).ConvertRequest(c, 0, req)
			require.NoError(t, err)
			got, ok := converted.(*model.GeneralOpenAIRequest)
			require.True(t, ok)
			require.Equal(t, tc.wantEffort, got.ReasoningEffort)
			require.Nil(t, got.Reasoning)
		})
	}
}

// TestGroqQwen38VisionConversion checks that image content survives conversion using t and returns no value.
func TestGroqQwen38VisionConversion(t *testing.T) {
	t.Parallel()

	const input = `{"model":"qwen/qwen3.8-27b","messages":[{"role":"user","content":[{"type":"text","text":"Describe this image."},{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]}],"reasoning_effort":"none"}`
	var req model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(input), &req))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	converted, err := (&Adaptor{}).ConvertRequest(c, 0, &req)
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)

	var want, got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(input), &want))
	require.NoError(t, json.Unmarshal(encoded, &got))
	require.JSONEq(t, string(want["messages"]), string(got["messages"]))
	require.JSONEq(t, string(want["reasoning_effort"]), string(got["reasoning_effort"]))
}
