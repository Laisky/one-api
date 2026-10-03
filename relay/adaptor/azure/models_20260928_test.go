package azure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestFoundrySeptember2026Catalog checks the five previously omitted IDs and
// standard Foundry token prices through public methods. It takes a test handle
// and returns nothing; listing a gated model does not prove account entitlement.
func TestFoundrySeptember2026Catalog(t *testing.T) {
	t.Parallel()
	a := &Adaptor{}
	for _, tc := range []struct {
		id                              string
		input, output, read, five, hour float64
	}{
		{"claude-sonnet-5-5", 2, 10, 0.2, 2.5, 4},
		{"claude-opus-5-5", 4, 20, 0.2, 5, 8},
		{"claude-opus-5", 5, 25, 0.5, 6.25, 10},
		{"claude-fable-5-1", 10, 50, 0.25, 12.5, 20},
		{"claude-mythos-5-1", 10, 50, 0.25, 12.5, 20},
	} {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, FoundryClaudeModels, tc.id)
			require.Contains(t, a.GetModelList(), tc.id)
			cfg, ok := a.GetDefaultModelPricing()[tc.id]
			require.True(t, ok)
			require.InDelta(t, tc.input*ratio.MilliTokensUsd, a.GetModelRatio(tc.id), 1e-12)
			require.InDelta(t, tc.output*ratio.MilliTokensUsd, cfg.Ratio*a.GetCompletionRatio(tc.id), 1e-12)
			require.InDelta(t, tc.read*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
			require.InDelta(t, tc.five*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12)
			require.InDelta(t, tc.hour*ratio.MilliTokensUsd, cfg.CacheWrite1hRatio, 1e-12)
			require.EqualValues(t, 1000000, cfg.ContextLength)
			require.EqualValues(t, 128000, cfg.MaxOutputTokens)
			require.Empty(t, cfg.TimeWindows)
		})
	}
	seen := make(map[string]bool)
	for _, id := range a.GetModelList() {
		require.False(t, seen[id], "duplicate public model %s", id)
		seen[id] = true
	}
	for _, old := range []string{"claude-sonnet-5", "claude-fable-5", "claude-mythos-5", "claude-haiku-4-5"} {
		require.Contains(t, a.GetModelList(), old, "retain configured-model compatibility")
	}
	for _, invented := range []string{"claude-sonnet-5-5-latest", "claude-sonnet-5-5-20260928", "claude-mythos-preview"} {
		require.NotContains(t, a.GetModelList(), invented)
	}
	require.Equal(t, (&openai.Adaptor{}).GetModelRatio("gpt-4o-mini"), a.GetModelRatio("gpt-4o-mini"))
}

// TestFoundrySonnet55DeploymentRoutes checks native routing and API-key headers
// across client formats and mapped deployment names. It takes a test handle and
// returns nothing; Entra token issuance is intentionally outside this fixture.
func TestFoundrySonnet55DeploymentRoutes(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses"} {
		for _, deployment := range []string{"claude-sonnet-5-5", "production-assistant"} {
			t.Run(path+"/"+deployment, func(t *testing.T) {
				t.Parallel()
				m := &meta.Meta{ChannelType: channeltype.Azure, BaseURL: "https://example.services.ai.azure.com/",
					OriginModelName: "claude-sonnet-5-5", ActualModelName: deployment,
					RequestURLPath: path, APIKey: "test-foundry-key"}
				a := &Adaptor{}
				url, err := a.GetRequestURL(m)
				require.NoError(t, err)
				require.Equal(t, "https://example.services.ai.azure.com/anthropic/v1/messages", url)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, nil)
				req := httptest.NewRequest(http.MethodPost, url, nil)
				require.NoError(t, a.SetupRequestHeader(c, req, m))
				require.Equal(t, "test-foundry-key", req.Header.Get("x-api-key"))
				require.Equal(t, "2023-06-01", req.Header.Get("anthropic-version"))
				require.NotContains(t, url, "/openai/deployments/")
			})
		}
	}
}

// TestFoundrySonnet55ChatConversion checks the Azure dispatch path, rather than
// invoking the Anthropic helper directly. It takes a test handle and returns nothing.
func TestFoundrySonnet55ChatConversion(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(ctxkey.Meta, &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "claude-sonnet-5-5", ActualModelName: "claude-sonnet-5-5"})
	temperature, topP, topK := 0.5, 0.9, 20
	converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &model.GeneralOpenAIRequest{
		Model: "claude-sonnet-5-5", MaxTokens: 256, Temperature: &temperature, TopP: &topP, TopK: &topK,
		Messages: []model.Message{{Role: "user", Content: "Hello"}},
	})
	require.NoError(t, err)
	raw, err := json.Marshal(converted)
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.JSONEq(t, `"claude-sonnet-5-5"`, string(payload["model"]))
	require.JSONEq(t, "256", string(payload["max_tokens"]))
	for _, field := range []string{"temperature", "top_p", "top_k"} {
		require.NotContains(t, payload, field)
	}
}
