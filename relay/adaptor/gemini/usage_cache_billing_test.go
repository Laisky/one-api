package gemini

import (
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// newCacheBillingTestContext builds a minimal gin.Context backed by a recorder so the
// real Gemini handlers can run without a live server.
// Parameters: t coordinates the test lifecycle.
// Returns: the gin.Context and the recorder capturing the handler output.
func newCacheBillingTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

// TestHandlerCapturesCachedAndThoughtsTokens verifies the non-stream Gemini Handler
// preserves the cached prompt tokens (PromptTokensDetails.CachedTokens) and folds the
// thinking tokens into the completion total. Gemini's promptTokenCount already includes
// the cached portion, so PromptTokens stays at the full count while the cached bucket is
// surfaced for discounted re-pricing in quota.Compute.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestHandlerCapturesCachedAndThoughtsTokens(t *testing.T) {
	t.Parallel()

	c, _ := newCacheBillingTestContext(t)

	geminiBody := `{
		"candidates": [
			{
				"content": {"role": "model", "parts": [{"text": "answer"}]},
				"finishReason": "STOP"
			}
		],
		"usageMetadata": {
			"promptTokenCount": 50000,
			"cachedContentTokenCount": 48000,
			"candidatesTokenCount": 100,
			"thoughtsTokenCount": 200,
			"totalTokenCount": 50300
		}
	}`

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(geminiBody)),
	}

	apiErr, usage := Handler(c, resp, 12345, "gemini-2.5-flash")
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	// PromptTokens keeps Gemini's full promptTokenCount (which includes the cached portion).
	require.Equal(t, 50000, usage.PromptTokens)
	// Completion = candidates + thoughts.
	require.Equal(t, 300, usage.CompletionTokens)
	require.Equal(t, 50300, usage.TotalTokens)

	// The cached prompt tokens must be surfaced so quota.Compute can re-price them at CachedInputRatio.
	require.NotNil(t, usage.PromptTokensDetails, "PromptTokensDetails must be populated so cached tokens are billed at the cached rate")
	require.Equal(t, 48000, usage.PromptTokensDetails.CachedTokens)
}

// TestStreamHandlerCapturesUsageMetadata verifies the streaming Gemini handler surfaces
// the authoritative upstream usageMetadata (cached prompt tokens + reasoning/thoughts in
// completion) instead of discarding it. Gemini sends the complete cumulative totals in the
// final SSE chunk, so the handler must report those rather than a text-based estimate.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestStreamHandlerCapturesUsageMetadata(t *testing.T) {
	t.Parallel()

	c, _ := newCacheBillingTestContext(t)

	// First chunk carries partial text and no usageMetadata; the final chunk carries the
	// complete cumulative totals (Gemini reports running totals, not per-chunk deltas).
	chunk1 := `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello "}]}}]}`
	chunk2 := `{"candidates":[{"content":{"role":"model","parts":[{"text":"world"}]},"finishReason":"STOP"}],` +
		`"usageMetadata":{"promptTokenCount":50000,"cachedContentTokenCount":48000,"candidatesTokenCount":100,"thoughtsTokenCount":200,"totalTokenCount":50300}}`

	sse := "data: " + chunk1 + "\n\n" + "data: " + chunk2 + "\n\n" + "data: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}

	apiErr, responseText, usage := StreamHandler(c, resp)
	require.Nil(t, apiErr)
	require.Equal(t, "Hello world", responseText)

	require.NotNil(t, usage, "StreamHandler must surface the upstream usageMetadata")
	require.Equal(t, 50000, usage.PromptTokens)
	// Completion must include thoughts (reasoning) tokens, not just candidates.
	require.Equal(t, 300, usage.CompletionTokens)
	require.Equal(t, 50300, usage.TotalTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 48000, usage.PromptTokensDetails.CachedTokens)
}

// TestStreamHandlerFallsBackWithoutUsageMetadata verifies that when upstream omits
// usageMetadata entirely, StreamHandler reports no authoritative usage so the caller
// falls back to the local text-based estimate (preserving existing behavior).
// Parameters: t coordinates the test case execution. Returns: no values.
func TestStreamHandlerFallsBackWithoutUsageMetadata(t *testing.T) {
	t.Parallel()

	c, _ := newCacheBillingTestContext(t)

	chunk := `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}]}`
	sse := "data: " + chunk + "\n\n" + "data: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
	}

	apiErr, responseText, usage := StreamHandler(c, resp)
	require.Nil(t, apiErr)
	require.Equal(t, "hi", responseText)
	require.Nil(t, usage, "without upstream usageMetadata StreamHandler must signal fallback")
}

// TestCachedTokensBilledAtCachedRate proves the end-to-end billing effect: once the cached
// prompt tokens are surfaced (BUG 1 fix), quota.Compute prices the cached portion at the
// model's CachedInputRatio instead of the full input rate. It compares the quota with and
// without the cached bucket using the real Gemini pricing adaptor.
// Parameters: t coordinates the test case execution. Returns: no values.
func TestCachedTokensBilledAtCachedRate(t *testing.T) {
	t.Parallel()

	modelName := "gemini-2.5-flash"
	adaptor := &Adaptor{}

	modelRatio := adaptor.GetModelRatio(modelName)
	require.Greater(t, modelRatio, 0.0, "unexpected model ratio: %v", modelRatio)
	groupRatio := 1.0

	promptTokens := 50000
	completionTokens := 300
	cachedPrompt := 48000

	baseUsage := &model.Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens}
	base := quotautil.Compute(quotautil.ComputeInput{
		Usage:          baseUsage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: adaptor,
	})

	eff := pricing.ResolveEffectivePricing(modelName, promptTokens, adaptor)
	require.Greater(t, eff.CachedInputRatio, 0.0, "gemini-2.5-flash must define a cached input ratio")
	normalInputPrice := base.UsedModelRatio * groupRatio
	cachedInputPrice := eff.CachedInputRatio * groupRatio
	require.Less(t, cachedInputPrice, normalInputPrice, "cached input must be cheaper than full input")

	cachedUsage := &model.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		PromptTokensDetails: &model.UsagePromptTokensDetails{
			CachedTokens: cachedPrompt,
		},
	}
	cached := quotautil.Compute(quotautil.ComputeInput{
		Usage:          cachedUsage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: adaptor,
	})

	// The cached bucket should reduce the quota by exactly (full - cached) price per cached token.
	expectedDelta := int64(math.Ceil(float64(cachedPrompt) * (cachedInputPrice - normalInputPrice)))
	actualDelta := cached.TotalQuota - base.TotalQuota
	require.InDelta(t, expectedDelta, actualDelta, 2,
		"cached tokens must be discounted: base=%d cached=%d", base.TotalQuota, cached.TotalQuota)
	require.Less(t, cached.TotalQuota, base.TotalQuota, "cached billing must cost less than full-price billing")
	require.Equal(t, cachedPrompt, cached.CachedPromptTokens)
}
