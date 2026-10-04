package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/Laisky/one-api/common/config"
	"github.com/stretchr/testify/require"
)

// TestClaudePDFTokenArithmetic compares the reservation to an independent
// arbitrary-precision oracle across rounding, wide-product and overflow edges.
func TestClaudePDFTokenArithmetic(t *testing.T) {
	for _, size := range []int{0, 1, 15, 16, 17, 1023, 1024, 1025, 65536, 65537, 1 << 20, 32 << 20} {
		t.Run(fmt.Sprintf("bytes_%d", size), func(t *testing.T) {
			got, err := claudePDFTokensForBytes(size, 64)
			require.NoError(t, err)
			require.Equal(t, claudePDFBigIntegerOracle(size, 64), got)
		})
	}
	got, err := claudePDFTokensForBytes(32<<20, 64)
	require.NoError(t, err)
	require.Greater(t, got, 1<<20, "document estimates must grow beyond former allowance caps")
	got, err = claudePDFTokensForBytes(math.MaxInt, 1024)
	require.NoError(t, err, "an overflowing intermediate product must not reject a representable quotient")
	require.Equal(t, math.MaxInt, got)
	for _, test := range []struct{ bytes, rate int }{
		{math.MaxInt, 1025}, {1025, math.MaxInt}, {-1, 64}, {1, 0}, {1, -1}, {0, 0},
	} {
		t.Run(fmt.Sprintf("invalid_bytes_%d_rate_%d", test.bytes, test.rate), func(t *testing.T) {
			_, err := claudePDFTokensForBytes(test.bytes, test.rate)
			require.Error(t, err)
		})
	}
}

// claudePDFBigIntegerOracle calculates the exact rational ceiling independently
// of the bounded production integer implementation.
func claudePDFBigIntegerOracle(decodedBytes, rate int) int {
	product := new(big.Int).Mul(big.NewInt(int64(decodedBytes)), big.NewInt(int64(rate)))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(product, big.NewInt(1024), remainder)
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	return int(quotient.Int64())
}

// TestClaudePDFDecodedBytes validates exact padded lengths and quote-private
// whitespace handling, while rejecting malformed or empty source data.
func TestClaudePDFDecodedBytes(t *testing.T) {
	for _, size := range []int{1, 2, 3, 15, 16, 17, 1023, 1024, 1025} {
		encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", size)))
		decorated := " \t\r\n" + strings.Join(strings.Split(encoded, ""), " \t\r\n") + "\r\n\t "
		for _, data := range []string{encoded, decorated} {
			got, err := claudePDFDecodedBytes(data)
			require.NoError(t, err, "bytes=%d", size)
			require.Equal(t, size, got)
		}
	}
	for _, data := range []string{"", " \t\r\n", "YQ", "YQ=", "YQ===", "YQ==junk", "YQ==Yg==", "====", "a!bc", "YQ==\v"} {
		t.Run(fmt.Sprintf("invalid_%q", data), func(t *testing.T) {
			_, err := claudePDFDecodedBytes(data)
			require.Error(t, err)
		})
	}
}

// TestClaudePDFNativeOverflowPropagation rejects both document-sum and final
// metadata-sum overflow and keeps quote-only normalization out of the payload.
func TestClaudePDFNativeOverflowPropagation(t *testing.T) {
	original := config.ClaudeNativePDFTokensPerKiB
	t.Cleanup(func() { config.ClaudeNativePDFTokensPerKiB = original })
	config.ClaudeNativePDFTokensPerKiB = math.MaxInt
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 1024)))
	request := documentQuoteRequest(map[string]any{"type": "base64", "media_type": "application/pdf", "data": encoded}, false)
	one, err := countClaudeNativeDocumentAllowance(request)
	require.NoError(t, err)
	require.Equal(t, math.MaxInt, one)
	_, err = getClaudeMessagesPromptTokens(context.Background(), request)
	require.Error(t, err, "metadata plus a maximum representable document must reject before wrapping")
	block := request.Messages[0].Content.([]any)[0]
	request.Messages[0].Content = []any{block, block}
	_, err = countClaudeNativeDocumentAllowance(request)
	require.Error(t, err, "multiple document estimates must reject before wrapping")
	_, err = getClaudeMessagesPromptTokens(context.Background(), request)
	require.Error(t, err)

	config.ClaudeNativePDFTokensPerKiB = 64
	request = documentQuoteRequest(map[string]any{"type": "base64", "media_type": "application/pdf", "data": " \tY Q = =\r\n"}, true)
	before, err := json.Marshal(request)
	require.NoError(t, err)
	got, err := countClaudeNativeDocumentAllowance(request)
	require.NoError(t, err)
	require.Equal(t, 1, got)
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, before, after, "normalizing source data must remain private to the quotation")
}
