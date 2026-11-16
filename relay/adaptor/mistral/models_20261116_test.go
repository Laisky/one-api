package mistral_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
)

// TestMistralNovemberHostedModels verifies exact provider IDs, native prices and
// tool/vision payload preservation through chat conversion. It takes t and
// returns no values without making authenticated provider requests.
func TestMistralNovemberHostedModels(t *testing.T) {
	t.Parallel()
	for id, rates := range map[string][3]float64{
		"zai-glm-5-3-flash":        {.15, .03, .50},
		"deepseek-v4-1-flash-1017": {.30, .007, 1.20},
	} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			a := &mistral.Adaptor{}
			cfg := a.GetDefaultModelPricing()[id]
			require.Contains(t, a.GetModelList(), id)
			require.InDelta(t, rates[0], a.GetModelRatio(id)/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, rates[1], cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
			require.InDelta(t, rates[2], a.GetModelRatio(id)*a.GetCompletionRatio(id)/ratio.MilliTokensUsd, 1e-12)
			require.EqualValues(t, 1_000_000, cfg.ContextLength)
			require.EqualValues(t, 128_000, cfg.MaxOutputTokens)
			require.Equal(t, []string{"text", "image"}, cfg.InputModalities)
			var request model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/image.png"}},{"type":"text","text":"Describe it"}]}],"tools":[{"type":"function","function":{"name":"record","parameters":{"type":"object"}}}],"tool_choice":"auto"}`), &request))
			request.Model = id
			converted, err := a.ConvertRequest(nil, 0, &request)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(encoded, &wire))
			require.Equal(t, id, wire["model"])
			require.Contains(t, string(encoded), "https://example.com/image.png")
			require.Contains(t, string(encoded), "record")
		})
	}
	latest := (&mistral.Adaptor{}).GetDefaultModelPricing()["mistral-large-latest"]
	require.Empty(t, latest.TimeWindows, "the former Large 3 alias must not inherit Large 4's expired launch discount")
	require.Equal(t, "high", latest.DefaultReasoningEffort)
	require.NotContains(t, latest.SupportedReasoningEfforts, "none")
}

// TestVoxtral3AudioTariffs verifies duration and character billing units
// and independent alias ownership while preserving pinned generation-2 prices.
// It accepts t and returns no values.
func TestVoxtral3AudioTariffs(t *testing.T) {
	t.Parallel()
	a := &mistral.Adaptor{}
	for prefix, price := range map[string]float64{"voxtral-mini-transcribe": .0035, "voxtral-small-transcribe": .004} {
		for _, suffix := range []string{"-3-0", "-3", "-latest"} {
			id := prefix + suffix
			require.Contains(t, a.GetModelList(), id)
			cfg := a.GetDefaultModelPricing()[id]
			require.NotNil(t, cfg.Audio)
			require.Equal(t, "seconds", cfg.Audio.InputUnit)
			require.InDelta(t, price, cfg.Audio.InputPriceUsd, 1e-12)
			require.EqualValues(t, 60, cfg.Audio.InputPriceQuantity)
			require.InDelta(t, price/60, cfg.Audio.UsdPerSecond, 1e-12)
		}
	}
	for _, suffix := range []string{"-3-0", "-3", "-latest"} {
		cfg := a.GetDefaultModelPricing()["voxtral-mini-tts"+suffix]
		require.NotNil(t, cfg.Audio)
		require.Equal(t, "characters", cfg.Audio.InputUnit)
		require.EqualValues(t, 13, cfg.Audio.InputPriceUsd)
		require.EqualValues(t, 1e6, cfg.Audio.InputPriceQuantity)
	}
	catalog := a.GetDefaultModelPricing()
	require.EqualValues(t, 16, catalog["voxtral-mini-tts-2603"].Audio.InputPriceUsd)
	require.EqualValues(t, .003, catalog["voxtral-mini-2602"].Audio.InputPriceUsd)
	require.NotSame(t, catalog["voxtral-mini-tts-3-0"].Audio, catalog["voxtral-mini-tts-latest"].Audio)
	require.NotSame(t, catalog["voxtral-mini-transcribe-3"].Audio, catalog["voxtral-mini-transcribe-latest"].Audio)
}
