package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestClaudeNativePDFPageTokenArithmetic pins the page cap, the context-window
// ceiling and invalid-input rejection of the per-request PDF estimate.
func TestClaudeNativePDFPageTokenArithmetic(t *testing.T) {
	for _, test := range []struct{ pages, perPage, want int }{
		{0, 8684, 0}, {1, 8684, 8684}, {115, 8684, 998660}, {116, 8684, 1_000_000},
		{600, 8684, 1_000_000}, {601, 1, 600}, {600, 1000, 600_000}, {1, 1048576, 1_000_000},
	} {
		t.Run(fmt.Sprintf("pages_%d_per_page_%d", test.pages, test.perPage), func(t *testing.T) {
			got, err := claudeNativePDFPageTokens(test.pages, test.perPage)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
	for _, test := range []struct{ pages, perPage int }{{-1, 1}, {1, 0}, {1, -1}, {1, math.MaxInt}} {
		t.Run(fmt.Sprintf("invalid_pages_%d_per_page_%d", test.pages, test.perPage), func(t *testing.T) {
			_, err := claudeNativePDFPageTokens(test.pages, test.perPage)
			require.Error(t, err)
		})
	}
}

// TestClaudePDFDecode validates exact padded lengths, quote-private whitespace
// handling and the in-memory limit, while rejecting malformed or empty data.
func TestClaudePDFDecode(t *testing.T) {
	for _, size := range []int{1, 2, 3, 15, 16, 17, 1023, 1024, 1025} {
		raw := strings.Repeat("x", size)
		encoded := base64.StdEncoding.EncodeToString([]byte(raw))
		decorated := " \t\r\n" + strings.Join(strings.Split(encoded, ""), " \t\r\n") + "\r\n\t "
		for _, data := range []string{encoded, decorated} {
			decoded, got, err := claudePDFDecode(data, 1<<20)
			require.NoError(t, err, "bytes=%d", size)
			require.Equal(t, size, got)
			require.Equal(t, raw, string(decoded))
			decoded, got, err = claudePDFDecode(data, size-1)
			require.NoError(t, err)
			require.Nil(t, decoded, "documents above the limit are validated without being retained")
			require.Equal(t, size, got)
		}
	}
	for _, data := range []string{"", " \t\r\n", "YQ", "YQ=", "YQ===", "YQ==junk", "YQ==Yg==", "====", "a!bc", "YQ==\v"} {
		for _, limit := range []int{0, 1 << 20} {
			t.Run(fmt.Sprintf("invalid_%q_limit_%d", data, limit), func(t *testing.T) {
				_, _, err := claudePDFDecode(data, limit)
				require.Error(t, err)
			})
		}
	}
}

// TestClaudeNativeDocumentSumOverflow rejects both document-sum and final
// metadata-sum overflow, and keeps quote-only whitespace normalization out of
// the provider payload.
func TestClaudeNativeDocumentSumOverflow(t *testing.T) {
	original := config.ClaudeNativeDocumentTokenAllowance
	t.Cleanup(func() { config.ClaudeNativeDocumentTokenAllowance = original })
	config.ClaudeNativeDocumentTokenAllowance = math.MaxInt
	request := documentQuoteRequest(map[string]any{"type": "file", "file_id": "file_synthetic"}, false)
	one, err := countClaudeNativeDocumentAllowance(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, math.MaxInt, one)
	_, err = getClaudeMessagesPromptTokens(context.Background(), request)
	require.Error(t, err, "metadata plus a maximum representable document must reject before wrapping")
	block := request.Messages[0].Content.([]any)[0]
	pdf := map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": "application/pdf",
		"data": base64.StdEncoding.EncodeToString(make([]byte, 17))}}
	for _, content := range [][]any{{block, block}, {block, pdf}} {
		request.Messages[0].Content = content
		_, err = countClaudeNativeDocumentAllowance(context.Background(), request)
		require.Error(t, err, "multiple document estimates must reject before wrapping")
	}

	config.ClaudeNativeDocumentTokenAllowance = original
	data := claudePDFPagePolicyFixture(t, 2, true)
	decorated := " \t" + data[:8] + "\r\n" + data[8:] + "\n"
	request = documentQuoteRequest(map[string]any{"type": "base64", "media_type": "application/pdf", "data": decorated}, true)
	before, err := json.Marshal(request)
	require.NoError(t, err)
	got, err := countClaudeNativeDocumentAllowance(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, documentedPDFPageQuote(2), got)
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, before, after, "normalizing source data must remain private to the quotation")
}
