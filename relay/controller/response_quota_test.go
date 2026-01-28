package controller

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
)

func TestCalculateResponseAPIPreconsumeQuotaBackground(t *testing.T) {
	t.Parallel()
	maxOutput := 1000
	inputRatio := 1.0
	completionMultiplier := 2.0
	outputRatio := inputRatio * completionMultiplier

	quota := calculateResponseAPIPreconsumeQuota(200, &maxOutput, inputRatio, outputRatio, true)

	expectedMin := int64(math.Ceil(float64(config.PreconsumeTokenForBackgroundRequest) * outputRatio))
	require.GreaterOrEqual(t, quota, expectedMin, "expected quota to be at least %d when background is enabled", expectedMin)
}

func TestCalculateResponseAPIPreconsumeQuotaForeground(t *testing.T) {
	t.Parallel()
	maxOutput := 500
	inputRatio := 1.0
	outputRatio := inputRatio

	quota := calculateResponseAPIPreconsumeQuota(200, &maxOutput, inputRatio, outputRatio, false)

	expected := int64(float64(200+maxOutput) * inputRatio)
	require.Equal(t, expected, quota, "expected quota %d for foreground request", expected)
}

func TestCalculateResponseAPIPreconsumeQuotaBackgroundLargeEstimate(t *testing.T) {
	t.Parallel()
	maxOutput := 55000
	inputRatio := 1.0
	completionMultiplier := 0.5
	outputRatio := inputRatio * completionMultiplier

	quota := calculateResponseAPIPreconsumeQuota(100, &maxOutput, inputRatio, outputRatio, true)

	expectedBase := int64(float64(100)*inputRatio + float64(maxOutput)*outputRatio)
	expectedMin := int64(math.Ceil(float64(config.PreconsumeTokenForBackgroundRequest) * outputRatio))
	if expectedBase < expectedMin {
		require.Equal(t, expectedMin, quota, "expected quota to match background floor")
	} else {
		require.Equal(t, expectedBase, quota, "expected quota to remain base estimate when it exceeds background floor %d", expectedMin)
	}
}
