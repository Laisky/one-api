package controller

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestCalculateImageBaseQuotaPerImage(t *testing.T) {
	t.Parallel()
	baseQuota := calculateImageBaseQuota(0.005, 0, 7.2, 1.0, 2)
	require.Equal(t, int64(36000), baseQuota)
}

func TestFinalizeImageQuotaAddsTokens(t *testing.T) {
	t.Parallel()
	usage := &relaymodel.Usage{
		PromptTokens:     437,
		CompletionTokens: 6208,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{
			TextTokens:  49,
			ImageTokens: 388,
		},
	}

	baseQuota := calculateImageBaseQuota(0.005, 0, 7.2, 1.0, 1)
	summary := finalizeImageQuota(baseQuota, true, "gpt-image-1-mini", "gpt-image-1-mini", usage, 1.0)

	expectedTokensFloat := computeImageUsageQuota("gpt-image-1-mini", usage, 1.0)
	expectedTokens := int64(math.Ceil(expectedTokensFloat))

	require.Equal(t, baseQuota, summary.BaseQuota)
	require.Equal(t, expectedTokens, summary.TokenQuota)
	require.Equal(t, baseQuota+expectedTokens, summary.TotalQuota)
	require.InDelta(t, expectedTokensFloat, summary.TokenQuotaFloat, 1e-6)
}

func TestFinalizeImageQuotaNoUsageKeepsBase(t *testing.T) {
	t.Parallel()
	baseQuota := calculateImageBaseQuota(0.005, 0, 7.2, 1.0, 1)
	summary := finalizeImageQuota(baseQuota, true, "gpt-image-1-mini", "gpt-image-1-mini", nil, 1.0)

	require.Equal(t, baseQuota, summary.TotalQuota)
	require.Equal(t, int64(0), summary.TokenQuota)
}

func TestFormatImageBillingLogIncludesDetails(t *testing.T) {
	t.Parallel()
	params := imageBillingLogParams{
		OriginModel:     "gpt-image-1-mini",
		Model:           "gpt-image-1-mini",
		Size:            "1024x1024",
		Quality:         "high",
		RequestCount:    1,
		BilledCount:     1,
		ImagePriceUsd:   0.005,
		ImageTier:       7.2,
		BaseQuota:       18000,
		TokenQuota:      25366,
		TotalQuota:      43366,
		GroupRatio:      1.0,
		TokenQuotaFloat: float64(25366),
	}

	logLine := formatImageBillingLog(params)
	require.Contains(t, logLine, "model=gpt-image-1-mini")
	require.Contains(t, logLine, "size=1024x1024")
	require.Contains(t, logLine, "quality=high")
	require.Contains(t, logLine, "requested_n=1")
	require.Contains(t, logLine, "billed_n=1")
	require.Contains(t, logLine, "group_rate=1.00")
	require.Contains(t, logLine, "unit_usd=0.0360")
	require.Contains(t, logLine, "base_usd=0.0360")
	require.Contains(t, logLine, "token_usd=0.0507")
	require.Contains(t, logLine, "total_usd=0.0867")
}
