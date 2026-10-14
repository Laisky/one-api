package vertexai

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiOctober13ArgonVertexCatalog checks inherited global-endpoint net
// tariffs and prevents fallback billing. Parameter t is the test handle; it
// returns no values. Cloud introductory prices are delivered as credits.
func TestGeminiOctober13ArgonVertexCatalog(t *testing.T) {
	adapter := &Adaptor{}
	require.Contains(t, adapter.GetModelList(), "gemini-4-argon")
	cfg, exists := adapter.GetDefaultModelPricing()["gemini-4-argon"]
	require.True(t, exists)
	require.Equal(t, int32(1_048_576), cfg.ContextLength)
	require.Equal(t, int32(32_768), cfg.MaxOutputTokens, "shared defaults must retain the conservative non-streaming REST limit")
	require.InDelta(t, 2*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
	require.InDelta(t, 10*ratio.MilliTokensUsd, cfg.Ratio*cfg.CompletionRatio, 1e-12)
	resolved := pricing.ApplyTimeWindow(cfg, time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC))
	require.InDelta(t, 4*ratio.MilliTokensUsd, resolved.Ratio, 1e-12)
	require.InDelta(t, 20*ratio.MilliTokensUsd, resolved.Ratio*resolved.CompletionRatio, 1e-12)
	require.InDelta(t, 0.20*ratio.MilliTokensUsd, resolved.CachedInputRatio, 1e-12)
}

// TestGeminiOctober13ArgonVertexURLs verifies existing global routing selects
// the Argon ID without remapping it. Parameter t is the test handle; it returns
// no values and performs no network calls.
func TestGeminiOctober13ArgonVertexURLs(t *testing.T) {
	require.True(t, IsRequireGlobalEndpoint("gemini-4-argon"))
	for _, tt := range []struct {
		name   string
		stream bool
		action string
	}{
		{"synchronous", false, "generateContent"},
		{"streaming", true, "streamGenerateContent?alt=sse"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requestMeta := &meta.Meta{
				ActualModelName: "gemini-4-argon",
				Mode:            relaymode.ChatCompletions,
				IsStream:        tt.stream,
			}
			requestMeta.Config.VertexAIProjectID = "argon-regression-project"
			url, err := (&Adaptor{}).GetRequestURL(requestMeta)
			require.NoError(t, err)
			require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/argon-regression-project/locations/global/publishers/google/models/gemini-4-argon:"+tt.action, url)
		})
	}
}
