package config

import (
	"math"
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeNativePDFTokensPerKiBConfiguration validates an operator-only
// positive integer rate without imposing a document-token cap.
func TestClaudeNativePDFTokensPerKiBConfiguration(t *testing.T) {
	value, err := parseClaudeNativePDFTokensPerKiB("ignored when unset", false)
	require.NoError(t, err)
	require.Equal(t, 64, value, "default remains a calibration estimate")
	require.Equal(t, DefaultClaudeNativePDFTokensPerKiB, value)
	for _, test := range []struct {
		raw  string
		want int
	}{
		{"1", 1}, {"64", 64}, {" 128 \t", 128}, {"+64", 64},
		{"1048577", 1048577}, {strconv.Itoa(math.MaxInt), math.MaxInt},
	} {
		t.Run("valid_"+test.raw, func(t *testing.T) {
			value, err := parseClaudeNativePDFTokensPerKiB(test.raw, true)
			require.NoError(t, err)
			require.Equal(t, test.want, value)
		})
	}
	overflow := new(big.Int).Add(big.NewInt(int64(math.MaxInt)), big.NewInt(1)).String()
	for _, raw := range []string{"", " \t", "0", "-1", "1.5", "1e3", "NaN", "Inf", "64tokens", overflow} {
		t.Run("invalid_"+raw, func(t *testing.T) {
			_, err := parseClaudeNativePDFTokensPerKiB(raw, true)
			require.Error(t, err)
		})
	}
}
