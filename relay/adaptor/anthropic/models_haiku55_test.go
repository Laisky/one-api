package anthropic_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestHaiku55CatalogAndPricing checks published IDs, metadata and real tier
// resolution. It takes a test handle and returns nothing; no provider is called.
func TestHaiku55CatalogAndPricing(t *testing.T) {
	t.Parallel()
	a := &anthropic.Adaptor{}
	cfg, ok := a.GetDefaultModelPricing()["claude-haiku-5-5"]
	require.True(t, ok)
	require.Contains(t, a.GetModelList(), "claude-haiku-5-5")
	require.EqualValues(t, 1000000, cfg.ContextLength)
	require.EqualValues(t, 128000, cfg.MaxOutputTokens)
	require.Zero(t, cfg.MaxReasoningTokens)
	require.Equal(t, "medium", cfg.DefaultReasoningEffort)
	require.ElementsMatch(t, []string{"low", "medium", "high", "xhigh", "max"}, cfg.SupportedReasoningEfforts)
	require.ElementsMatch(t, []string{"text", "image", "file"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	require.ElementsMatch(t, []string{"stop", "max_tokens"}, cfg.SupportedSamplingParameters)
	require.Contains(t, cfg.SupportedFeatures, "reasoning")
	require.Contains(t, cfg.SupportedFeatures, "tools")
	require.Len(t, cfg.Tiers, 1)
	require.Empty(t, cfg.TimeWindows)
	for _, name := range []string{"claude-haiku-5-5-latest", "claude-haiku-5-5-20261007", "claude-haiku-5-5@20261007", "anthropic.claude-haiku-5-5"} {
		require.NotContains(t, a.GetModelList(), name)
		require.False(t, anthropic.IsClaudeHaiku55(name))
	}
	require.True(t, anthropic.IsClaudeAdaptiveThinkingModel(" Claude-Haiku-5-5 "))
	require.False(t, anthropic.IsClaudeAdaptiveThinkingModel("claude-haiku-4-5"))
	for _, count := range []int{0, 99999, 100000, 100001, 900000} {
		eff := pricing.ResolveEffectivePricingForUsage("claude-haiku-5-5", count, 128000, a)
		factor, threshold := 1.0, 0
		if count > 100000 {
			factor, threshold = 5, 100001
		}
		require.Equal(t, threshold, eff.AppliedTierThreshold)
		require.Zero(t, eff.AppliedOutputTierThreshold)
		require.InDelta(t, factor*0.1*ratio.MilliTokensUsd, eff.InputRatio, 1e-12)
		require.InDelta(t, factor*0.5*ratio.MilliTokensUsd, eff.OutputRatio, 1e-12)
		require.InDelta(t, factor*0.01*ratio.MilliTokensUsd, eff.CachedInputRatio, 1e-12)
		require.InDelta(t, factor*0.125*ratio.MilliTokensUsd, eff.CacheWrite5mRatio, 1e-12)
		require.InDelta(t, factor*0.2*ratio.MilliTokensUsd, eff.CacheWrite1hRatio, 1e-12)
	}
	legacy, ok := a.GetDefaultModelPricing()["claude-haiku-4-5"]
	require.True(t, ok)
	require.InDelta(t, ratio.MilliTokensUsd, legacy.Ratio, 1e-12)
	require.InDelta(t, 0.1*ratio.MilliTokensUsd, legacy.CachedInputRatio, 1e-12)
	require.Empty(t, legacy.Tiers)
}

// TestHaiku55ConvertedControls exercises Chat conversion, effort translation and
// forced tool choice without mutating the caller. It takes t and returns nothing.
func TestHaiku55ConvertedControls(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"omitted", "enabled", "adaptive", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			var request model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(`{"model":"claude-haiku-5-5","max_tokens":256,"temperature":0.5,"top_p":0.9,"top_k":40,"reasoning_effort":"high","tool_choice":"required","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"messages":[{"role":"user","content":"Hello"}]}`), &request))
			if mode != "omitted" {
				budget := 2048
				request.Thinking = &model.Thinking{Type: mode, BudgetTokens: &budget}
			}
			before, err := json.Marshal(request)
			require.NoError(t, err)
			converted, err := (&anthropic.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &request)
			require.NoError(t, err)
			body, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &payload))
			require.JSONEq(t, `{"type":"any"}`, string(payload["tool_choice"]))
			require.JSONEq(t, `{"effort":"high"}`, string(payload["output_config"]))
			require.Equal(t, "256", string(payload["max_tokens"]))
			for _, key := range []string{"temperature", "top_p", "top_k"} {
				require.NotContains(t, payload, key)
			}
			if mode == "omitted" {
				require.NotContains(t, payload, "thinking")
			} else if mode == "disabled" {
				require.JSONEq(t, `{"type":"disabled"}`, string(payload["thinking"]))
			} else {
				require.JSONEq(t, `{"type":"adaptive"}`, string(payload["thinking"]))
			}
			after, err := json.Marshal(request)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

// TestHaiku55NativeBody checks final HTTP-boundary controls and opaque signed
// content preservation. It takes t and returns nothing without dispatching HTTP.
func TestHaiku55NativeBody(t *testing.T) {
	t.Parallel()
	const raw = `{"model":"claude-haiku-5-5","max_tokens":256,"temperature":0.3,"top_p":0.8,"top_k":10,"thinking":{"type":"enabled","budget_tokens":2048,"display":"summarized"},"tool_choice":{"type":"tool","name":"lookup"},"future_extension":{"id":9007199254740993},"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"signed text","signature":"opaque-signature"}]},{"role":"user","content":"Continue"}]}`
	reader, err := anthropic.PrepareRequestBody(nil, "claude-haiku-5-5", strings.NewReader(raw))
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	var before, after map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &before))
	require.NoError(t, json.Unmarshal(body, &after))
	for _, key := range []string{"messages", "tool_choice", "future_extension"} {
		require.Equal(t, before[key], after[key], "preserve %s byte-for-byte", key)
	}
	require.JSONEq(t, `{"type":"adaptive","display":"summarized"}`, string(after["thinking"]))
	for _, key := range []string{"temperature", "top_p", "top_k"} {
		require.NotContains(t, after, key)
	}
	for _, controls := range []string{
		`"thinking":{"type":"disabled"},"output_config":{"effort":"high"}`,
		`"thinking":{"type":"adaptive"},"output_config":{"effort":"max"}`,
		`"thinking":{"type":"disabled"},"tool_choice":{"type":"any"}`,
	} {
		_, err := anthropic.PrepareRequestBody(nil, "claude-haiku-5-5", strings.NewReader(`{"messages":[{"role":"user","content":"Hi"}],`+controls+`}`))
		require.NoError(t, err)
	}
	for _, invalid := range []string{
		`null`, `[]`, `{`,
		`{"thinking":{"type":null}}`,
		`{"thinking":{"type":"between_tools"}}`,
		`{"thinking":{"type":"disabled"},"output_config":{"effort":"xhigh"}}`,
		`{"thinking":{"type":"disabled"},"output_config":{"effort":"max"}}`,
		`{"output_config":{"effort":null}}`,
		`{"output_config":{"effort":"unknown"}}`,
		`{"output_config":null}`,
		`{"messages":[{"role":"assistant","content":"Prefill"}]}`,
		`{"thinking":{"type":"disabled"},"messages":[{"role":"user","output_config":{"effort":"high"}}]}`,
	} {
		_, err := anthropic.PrepareRequestBody(nil, "claude-haiku-5-5", strings.NewReader(invalid))
		require.Error(t, err, invalid)
	}
	original := strings.NewReader(raw)
	unchanged, err := anthropic.PrepareRequestBody(nil, "unrelated-model", original)
	require.NoError(t, err)
	require.Same(t, original, unchanged)
	_, err = anthropic.PrepareRequestBody(nil, "claude-sonnet-5-5", strings.NewReader(raw))
	require.Error(t, err, "Sonnet's forced-tool restriction must remain independent")
}

// TestHaiku55AzureOrigin checks an opaque deployment uses the known origin's
// policy without rewriting its wire ID. It takes t and returns nothing.
func TestHaiku55AzureOrigin(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Meta, &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "claude-haiku-5-5"})
	reader, err := anthropic.PrepareRequestBody(c, "deployment-a", strings.NewReader(`{"model":"deployment-a","thinking":{"type":"disabled"},"temperature":0.3}`))
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"deployment-a","thinking":{"type":"disabled"}}`, string(body))
}
