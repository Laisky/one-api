package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCalculateRerankQuota verifies token-priced and per-call rerank settlement.
func TestCalculateRerankQuota(t *testing.T) {
	t.Parallel()

	require.EqualValues(t, 25, calculateRerankQuota(1000, 0.025, 1.0, false))
	require.EqualValues(t, 50, calculateRerankQuota(1000, 50, 1.0, true))
	require.EqualValues(t, 1, calculateRerankQuota(1, 0.001, 1.0, false))
	require.EqualValues(t, 0, calculateRerankQuota(1000, 0, 1.0, false))
	require.EqualValues(t, 0, calculateRerankQuota(-1, 1, 1.0, false))
}
