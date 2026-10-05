package config

import (
	"math"
	"math/big"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeNativePDFTokensPerPageConfiguration validates the operator-only
// per-page estimate and its documented default derivation.
func TestClaudeNativePDFTokensPerPageConfiguration(t *testing.T) {
	value, err := parseClaudeNativePDFTokensPerPage("ignored when unset", false)
	require.NoError(t, err)
	require.Equal(t, 3000*130/100+4784, value, "default is documented text (x1.3 tokenizer growth) plus the largest page image")
	require.Equal(t, DefaultClaudeNativePDFTokensPerPage, value)
	for _, test := range []struct {
		raw  string
		want int
	}{
		{"1", 1}, {"8684", 8684}, {" 4568 \t", 4568}, {"+1568", 1568}, {"1048576", 1048576},
	} {
		t.Run("valid_"+test.raw, func(t *testing.T) {
			value, err := parseClaudeNativePDFTokensPerPage(test.raw, true)
			require.NoError(t, err)
			require.Equal(t, test.want, value)
		})
	}
	overflow := new(big.Int).Add(big.NewInt(int64(math.MaxInt)), big.NewInt(1)).String()
	for _, raw := range []string{"", " \t", "0", "-1", "1.5", "1e3", "NaN", "Inf", "64tokens", "1048577", strconv.Itoa(math.MaxInt), overflow} {
		t.Run("invalid_"+raw, func(t *testing.T) {
			_, err := parseClaudeNativePDFTokensPerPage(raw, true)
			require.Error(t, err)
		})
	}
}
