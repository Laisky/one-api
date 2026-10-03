package openrouter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT61SolCatalog verifies provider-specific public IDs, prices, and metadata
// without promoting native OpenAI names, canonical slugs, or asynchronous Batch IDs.
func TestGPT61SolCatalog(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	for _, name := range []string{"openai/gpt-6.1-sol", "openai/gpt-6.1-sol-pro"} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, a.GetModelList(), name)
			cfg, ok := a.GetDefaultModelPricing()[name]
			require.True(t, ok)
			require.InDelta(t, 2.0, a.GetModelRatio(name)/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 10.0, a.GetModelRatio(name)*a.GetCompletionRatio(name)/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 0.1, cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 2.5, cfg.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-9)
			require.Zero(t, cfg.CacheWrite1hRatio)
			require.Equal(t, int32(1_050_000), cfg.ContextLength)
			require.Equal(t, int32(128_000), cfg.MaxOutputTokens)
			require.Equal(t, []string{"file", "image", "text"}, cfg.InputModalities)
			require.Equal(t, []string{"text"}, cfg.OutputModalities)
			require.Equal(t, []string{"max", "xhigh", "high", "medium", "low"}, cfg.SupportedReasoningEfforts)
			require.Equal(t, "medium", cfg.DefaultReasoningEffort)
			for _, feature := range []string{"tools", "json_mode", "structured_outputs", "reasoning"} {
				require.Contains(t, cfg.SupportedFeatures, feature)
			}
			for _, parameter := range []string{"temperature", "top_p", "logprobs"} {
				require.NotContains(t, cfg.SupportedSamplingParameters, parameter)
			}
			require.NotContains(t, a.GetModelList(), name+":batch")
			require.NotContains(t, a.GetModelList(), name+"-20260929")
		})
	}
	require.NotContains(t, a.GetModelList(), "gpt-6.1-sol")
	require.InDelta(t, 0.2, ModelRatios["openai/gpt-6-sol"].CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
	require.Contains(t, ModelRatios["openai/gpt-6-sol"].SupportedReasoningEfforts, "none")
}

// TestGPT61SolCatalogOwnership verifies both entries and repeated constructor
// calls own their mutable metadata; an alias edit must not alter other defaults.
func TestGPT61SolCatalogOwnership(t *testing.T) {
	t.Parallel()
	catalog := gpt61SolModels()
	base := catalog["openai/gpt-6.1-sol"]
	base.Tiers[0].Ratio = 999
	base.InputModalities[0] = "changed"
	base.OutputModalities[0] = "changed"
	base.SupportedFeatures[0] = "changed"
	base.SupportedSamplingParameters[0] = "changed"
	base.SupportedReasoningEfforts[0] = "changed"
	fresh := gpt61SolModels()
	require.Equal(t, fresh["openai/gpt-6.1-sol-pro"], catalog["openai/gpt-6.1-sol-pro"])
	require.Equal(t, fresh["openai/gpt-6.1-sol"], ModelRatios["openai/gpt-6.1-sol"])
	require.NotEqual(t, fresh["openai/gpt-6.1-sol"], base)
}

// TestGPT61SolOpenRouterTransport verifies both public aliases retain OpenRouter's
// Chat Completions surface and tools instead of inheriting native OpenAI routing.
func TestGPT61SolOpenRouterTransport(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"openai/gpt-6.1-sol", "openai/gpt-6.1-sol-pro"} {
		for _, path := range []string{"/v1/chat/completions", "/v1/messages"} {
			a := &Adaptor{}
			info := &meta.Meta{ChannelType: channeltype.OpenRouter, ActualModelName: name,
				BaseURL: "https://openrouter.ai/api", RequestURLPath: path}
			url, err := a.GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, "https://openrouter.ai/api/v1/chat/completions", url)
		}
		var request model.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal([]byte(`{
			"messages":[{"role":"user","content":"Find the release notes."}],
			"reasoning_effort":"max",
			"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}]
		}`), &request))
		request.Model = name
		a := &Adaptor{}
		converted, err := a.ConvertRequest(nil, relaymode.ChatCompletions, &request)
		require.NoError(t, err)
		require.Same(t, &request, converted)
		require.Equal(t, name, request.Model, "preserve the Pro alias sent to OpenRouter")
		require.Equal(t, "max", *request.ReasoningEffort)
		require.Len(t, request.Tools, 1)
		require.Equal(t, "lookup", request.Tools[0].Function.Name)
	}
}
