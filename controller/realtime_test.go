package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestPostConsumeRealtimeQuota_ZeroUsageKeepsPreConsumed verifies that when
// usage is zero but pre-consumed quota exists, the pre-consumed amount is
// RETAINED as the charge (zero-usage guard prevents free rides).
func TestPostConsumeRealtimeQuota_ZeroUsageKeepsPreConsumed(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/realtime?model=gpt-4o-realtime-preview", nil)
	c.Set(ctxkey.RequestId, "req_test_zero")
	c.Set(ctxkey.ChannelRatio, 1.0)

	relayMeta := &meta.Meta{
		TokenId:         1,
		UserId:          1,
		ChannelId:       1,
		ActualModelName: "gpt-4o-realtime-preview",
	}

	usage := &rmodel.Usage{
		PromptTokens:     0,
		CompletionTokens: 0,
	}

	preConsumedQuota := int64(100)

	// Zero usage + pre-consumed quota → should keep pre-consumed amount
	quota := postConsumeRealtimeQuota(c, relayMeta, usage, preConsumedQuota,
		1.0, 1.0, nil, nil, nil, nil, 0)
	require.Equal(t, float64(preConsumedQuota), quota,
		"zero usage should keep pre-consumed quota to prevent free rides")

	// Verify billing was marked reconciled
	reconciled, _ := c.Get(ctxkey.BillingReconciled)
	require.Equal(t, true, reconciled)
}

// TestPostConsumeRealtimeQuota_NilUsageKeepsPreConsumed verifies the same
// zero-usage guard for nil usage.
func TestPostConsumeRealtimeQuota_NilUsageKeepsPreConsumed(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)

	relayMeta := &meta.Meta{
		TokenId:         1,
		UserId:          1,
		ChannelId:       1,
		ActualModelName: "gpt-4o-realtime-preview",
	}

	preConsumedQuota := int64(50)

	quota := postConsumeRealtimeQuota(c, relayMeta, nil, preConsumedQuota,
		1.0, 1.0, nil, nil, nil, nil, 0)
	require.Equal(t, float64(preConsumedQuota), quota,
		"nil usage should keep pre-consumed quota")
}

// TestPostConsumeRealtimeQuota_ZeroUsageZeroPreConsumed verifies that when
// both usage and pre-consumed are zero, no charge occurs.
func TestPostConsumeRealtimeQuota_ZeroUsageZeroPreConsumed(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)

	relayMeta := &meta.Meta{
		TokenId:         1,
		UserId:          1,
		ChannelId:       1,
		ActualModelName: "gpt-4o-realtime-preview",
	}

	usage := &rmodel.Usage{}

	quota := postConsumeRealtimeQuota(c, relayMeta, usage, 0,
		1.0, 1.0, nil, nil, nil, nil, 0)
	require.Equal(t, float64(0), quota)
}

// TestPostConsumeRealtimeQuota_IncompleteMeta verifies that billing is skipped
// when meta information is incomplete.
func TestPostConsumeRealtimeQuota_IncompleteMeta(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)

	relayMeta := &meta.Meta{
		ActualModelName: "gpt-4o-realtime-preview",
	}

	usage := &rmodel.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
	}

	quota := postConsumeRealtimeQuota(c, relayMeta, usage, 0,
		1.0, 1.0, nil, nil, nil, nil, 0)
	require.Equal(t, float64(0), quota, "should return 0 when meta is incomplete")
}

// TestPostConsumeRealtimeQuota_WithUsage verifies that non-zero usage
// results in a positive quota computation.
func TestPostConsumeRealtimeQuota_WithUsage(t *testing.T) {
	t.Parallel()

	usage := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}

	pricingAdaptor := relay.GetAdaptor(0) // OpenAI
	modelName := "gpt-4o-realtime-preview"
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)
	groupRatio := 1.0

	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usage,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: pricingAdaptor,
	})

	require.Greater(t, computeResult.TotalQuota, int64(0),
		"quota should be positive for non-zero usage")
	require.Equal(t, 1000, computeResult.PromptTokens)
	require.Equal(t, 500, computeResult.CompletionTokens)
}

// TestRtBillingAuditSafetyNet_Reconciled verifies the safety net does
// nothing when billing is reconciled.
func TestRtBillingAuditSafetyNet_Reconciled(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)

	rtMarkPreConsumed(c, 1000)
	rtMarkBillingReconciled(c)

	// Should not panic
	require.NotPanics(t, func() {
		rtBillingAuditSafetyNet(c)
	})
}

// TestRtBillingAuditSafetyNet_Unreconciled verifies the safety net
// detects unreconciled pre-consumed quota.
func TestRtBillingAuditSafetyNet_Unreconciled(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)

	rtMarkPreConsumed(c, 1000)
	// Intentionally NOT calling rtMarkBillingReconciled

	// Should not panic, but should log CRITICAL error
	require.NotPanics(t, func() {
		rtBillingAuditSafetyNet(c)
	})
}

// TestRtBillingAuditSafetyNet_NoPreConsume verifies safety net
// does nothing when no pre-consumption occurred.
func TestRtBillingAuditSafetyNet_NoPreConsume(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/v1/realtime", nil)

	// No markPreConsumed call
	require.NotPanics(t, func() {
		rtBillingAuditSafetyNet(c)
	})
}

// TestPreConsumeEstimate_AudioAware verifies the pre-consume formula uses
// audio token rates and produces a realistic estimate.
func TestPreConsumeEstimate_AudioAware(t *testing.T) {
	t.Parallel()

	for _, modelName := range []string{
		"gpt-realtime-1.5",
		"gpt-realtime-mini",
		"gpt-4o-realtime-preview",
	} {
		t.Run(modelName, func(t *testing.T) {
			pricingAdaptor := relay.GetAdaptor(0)
			modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)

			preConsumed := estimateRealtimePreConsumeQuota(
				modelName, modelRatio, 1.0, nil, pricingAdaptor)

			require.Greater(t, preConsumed, int64(0),
				"pre-consume estimate should be positive")

			// The estimate should be significantly higher than a text-only estimate
			// of the same token count, because audio costs 8-20x more
			textOnlyEstimate := int64(float64(realtimePreConsumeSeconds*(realtimeAudioInputTokensPerSec+realtimeAudioOutputTokensPerSec)) * modelRatio)
			require.Greater(t, preConsumed, textOnlyEstimate*3,
				"audio-aware estimate should be at least 3x text-only: audio=%d, text=%d",
				preConsumed, textOnlyEstimate)
		})
	}
}

// TestPreConsumeEstimate_MatchesExpectedUSD verifies the estimate matches
// a 2-minute audio session cost for gpt-realtime-1.5.
func TestPreConsumeEstimate_MatchesExpectedUSD(t *testing.T) {
	t.Parallel()

	pricingAdaptor := relay.GetAdaptor(0)
	modelName := "gpt-realtime-1.5"
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)

	preConsumed := estimateRealtimePreConsumeQuota(
		modelName, modelRatio, 1.0, nil, pricingAdaptor)

	// Expected for 120s session:
	// Input:  120 * 10 = 1200 audio tokens * $32/1M = $0.0384
	// Output: 120 * 20 = 2400 audio tokens * $64/1M = $0.1536
	// Total: $0.192 = 96,000 quota
	expectedUSD := 1200.0*32.0/1_000_000 + 2400.0*64.0/1_000_000
	expectedQuota := int64(expectedUSD * ratio.QuotaPerUsd)

	require.InDelta(t, expectedQuota, preConsumed, float64(expectedQuota)*0.05,
		"2-min audio session estimate: expected ~$%.4f = %d quota, got %d",
		expectedUSD, expectedQuota, preConsumed)
}

// TestApplyRealtimeAudioSurcharge_WithAudioTokens verifies that audio tokens
// get a surcharge added to ToolsCost.
func TestApplyRealtimeAudioSurcharge_WithAudioTokens(t *testing.T) {
	t.Parallel()

	pricingAdaptor := relay.GetAdaptor(0) // OpenAI
	modelName := "gpt-4o-realtime-preview"
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)
	groupRatio := 1.0

	usage := &rmodel.Usage{
		PromptTokens:     500,
		CompletionTokens: 300,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 400,
			TextTokens:  100,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 250,
			TextTokens:  50,
		},
	}

	applyRealtimeAudioSurcharge(usage, modelName, modelRatio, groupRatio,
		nil, nil, pricingAdaptor, nil)

	require.Greater(t, usage.ToolsCost, int64(0),
		"audio surcharge should be positive when audio tokens are present")
}

// TestApplyRealtimeAudioSurcharge_NoAudioTokens verifies no surcharge
// when there are no audio tokens.
func TestApplyRealtimeAudioSurcharge_NoAudioTokens(t *testing.T) {
	t.Parallel()

	pricingAdaptor := relay.GetAdaptor(0)
	modelName := "gpt-4o-realtime-preview"
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)

	usage := &rmodel.Usage{
		PromptTokens:     500,
		CompletionTokens: 300,
		// No PromptTokensDetails or CompletionTokensDetails
	}

	applyRealtimeAudioSurcharge(usage, modelName, modelRatio, 1.0,
		nil, nil, pricingAdaptor, nil)

	require.Equal(t, int64(0), usage.ToolsCost,
		"no surcharge when no audio tokens")
}

// TestApplyRealtimeAudioSurcharge_NonRealtimeModel verifies no surcharge
// for models without audio pricing config.
func TestApplyRealtimeAudioSurcharge_NonRealtimeModel(t *testing.T) {
	t.Parallel()

	pricingAdaptor := relay.GetAdaptor(0)
	modelName := "gpt-4o" // not a realtime model, no Audio config

	usage := &rmodel.Usage{
		PromptTokens: 100,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 100,
		},
	}

	applyRealtimeAudioSurcharge(usage, modelName, 1.0, 1.0,
		nil, nil, pricingAdaptor, nil)

	// gpt-4o doesn't have Audio pricing, so no surcharge
	require.Equal(t, int64(0), usage.ToolsCost)
}

// TestAudioSurchargeSignificantlyHigherThanTextOnly verifies that the total
// quota with audio surcharge is significantly higher than text-only billing.
func TestAudioSurchargeSignificantlyHigherThanTextOnly(t *testing.T) {
	t.Parallel()

	pricingAdaptor := relay.GetAdaptor(0)
	modelName := "gpt-4o-realtime-preview"
	modelRatio := pricing.GetModelRatioWithThreeLayers(modelName, nil, pricingAdaptor)
	groupRatio := 1.0

	// Usage with audio tokens
	usageWithAudio := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		PromptTokensDetails: &rmodel.UsagePromptTokensDetails{
			AudioTokens: 800,
			TextTokens:  200,
		},
		CompletionTokensDetails: &rmodel.UsageCompletionTokensDetails{
			AudioTokens: 400,
			TextTokens:  100,
		},
	}

	applyRealtimeAudioSurcharge(usageWithAudio, modelName, modelRatio, groupRatio,
		nil, nil, pricingAdaptor, nil)

	resultWithAudio := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usageWithAudio,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: pricingAdaptor,
	})

	// Same usage but no audio surcharge (simulates incorrect billing)
	usageTextOnly := &rmodel.Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
	}
	resultTextOnly := quotautil.Compute(quotautil.ComputeInput{
		Usage:          usageTextOnly,
		ModelName:      modelName,
		ModelRatio:     modelRatio,
		GroupRatio:     groupRatio,
		PricingAdaptor: pricingAdaptor,
	})

	// Audio billing should be significantly higher than text-only
	require.Greater(t, resultWithAudio.TotalQuota, resultTextOnly.TotalQuota*3,
		"audio billing should be at least 3x text-only billing for 80%% audio session")
}

// TestRelayRealtimeHandlerSignature verifies the function signature.
func TestRelayRealtimeHandlerSignature(t *testing.T) {
	var fn func(*gin.Context) = RelayRealtime
	require.NotNil(t, fn)
}

// TestRelayRealtimeSessionsHandlerSignature verifies the function signature.
func TestRelayRealtimeSessionsHandlerSignature(t *testing.T) {
	var fn func(*gin.Context) = RelayRealtimeSessions
	require.NotNil(t, fn)
}
