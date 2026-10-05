package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// claudePDFPagePolicyFixture returns base64 data for a valid generated PDF.
func claudePDFPagePolicyFixture(t *testing.T, pages int, objectStream bool) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(claudePDFPageFixture(t, claudePDFPageFixtureOptions{
		Pages: pages, ObjectStream: objectStream, LineText: "Synthetic policy page text.", Lines: 3,
	}))
}

// requireClaudePDFPageQuote quotes a request through the production prepared
// native path and asserts that the payload is preserved.
func requireClaudePDFPageQuote(t *testing.T, request *ClaudeMessagesRequest) int {
	t.Helper()
	before, err := json.Marshal(request)
	require.NoError(t, err)
	quote, err := preparedClaudePromptTokens(context.Background(), request, nil)
	require.NoError(t, err)
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, before, after, "quoting must preserve transport bytes and caller ownership")
	return quote
}

// TestClaudeNativePDFPagePolicy quotes documented per-page cost for valid PDFs
// across the page-limit and context-window boundaries.
func TestClaudeNativePDFPagePolicy(t *testing.T) {
	for _, pages := range []int{1, 2, 7, 115, 116, 600, 601} {
		for _, objectStream := range []bool{false, true} {
			t.Run(fmt.Sprintf("pages=%d/object_stream=%v", pages, objectStream), func(t *testing.T) {
				documents := []any{claudePDFPageDocument(claudePDFPagePolicyFixture(t, pages, objectStream))}
				quote := requireClaudePDFPageQuote(t, claudePDFPageRequest(documents, false))
				want := claudePDFPageMetadataTokens(t, documents) + documentedPDFPageQuote(pages)
				t.Logf("PDF_PAGE_POLICY pages=%d object_stream=%v quote=%d documented=%d", pages, objectStream, quote, want)
				require.Equal(t, want, quote)
			})
		}
	}
}

// TestClaudeNativePDFPageStructure sums pages across documents (directly or in
// tool results), retains metadata charges and applies one per-request cap.
func TestClaudeNativePDFPageStructure(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, metadata := range []bool{false, true} {
			t.Run(fmt.Sprintf("nested=%v/metadata=%v", nested, metadata), func(t *testing.T) {
				documents := []any{
					claudePDFPageDocument(claudePDFPagePolicyFixture(t, 3, false)),
					claudePDFPageDocument(claudePDFPagePolicyFixture(t, 5, true)),
				}
				if metadata {
					documents[0].(map[string]any)["title"] = strings.Repeat("Synthetic title ", 40)
					documents[1].(map[string]any)["context"] = strings.Repeat("Synthetic context ", 60)
					documents[1].(map[string]any)["citations"] = map[string]any{"enabled": true}
				}
				quote := requireClaudePDFPageQuote(t, claudePDFPageRequest(documents, nested))
				require.Equal(t, claudePDFPageMetadataTokens(t, documents)+documentedPDFPageQuote(8), quote,
					"pages from every document add before the per-page estimate applies")
			})
		}
	}
	documents := []any{
		claudePDFPageDocument(claudePDFPagePolicyFixture(t, 400, true)),
		claudePDFPageDocument(claudePDFPagePolicyFixture(t, 400, true)),
		claudePDFPageDocument(claudePDFPagePolicyFixture(t, 1, false)),
	}
	quote := requireClaudePDFPageQuote(t, claudePDFPageRequest(documents, false))
	require.Equal(t, claudePDFPageMetadataTokens(t, documents)+documentedPDFContextCeiling, quote,
		"the documented page limit and context window cap the request, not each document")
}

// TestClaudeNativePDFPageBase64Policy keeps quote-private whitespace handling
// and rejects malformed or empty PDF source data before dispatch.
func TestClaudeNativePDFPageBase64Policy(t *testing.T) {
	valid := claudePDFPagePolicyFixture(t, 2, false)
	for _, tc := range []struct {
		name string
		data any
		bad  bool
	}{
		{name: "canonical", data: valid},
		{name: "crlf", data: valid[:4] + "\r\n" + valid[4:] + "\n"},
		{name: "space_tab", data: valid[:4] + " \t" + valid[4:]},
		{name: "empty", data: "", bad: true},
		{name: "padding", data: "AAA", bad: true},
		{name: "trailing_junk", data: valid + "!", bad: true},
		{name: "wrong_type", data: 17, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := claudePDFPageDocument("")
			document["source"].(map[string]any)["data"] = tc.data
			documents := []any{document}
			quote, err := preparedClaudePromptTokens(context.Background(), claudePDFPageRequest(documents, false), nil)
			t.Logf("PDF_PAGE_BASE64 case=%s quote=%d error=%v", tc.name, quote, err)
			if tc.bad {
				require.Error(t, err, "invalid source must fail before paid dispatch rather than reserve zero or a flat allowance")
				return
			}
			require.NoError(t, err)
			require.Equal(t, claudePDFPageMetadataTokens(t, documents)+documentedPDFPageQuote(2), quote)
		})
	}
}

// TestClaudeNativePDFPageOpaqueControls keeps the separate URL, file and
// non-PDF allowance without remote fetching, and bounds bytes that show no page
// evidence at the documented per-request maximum.
func TestClaudeNativePDFPageOpaqueControls(t *testing.T) {
	for _, source := range []map[string]any{
		{"type": "url", "url": "https://example.invalid/synthetic.pdf"},
		{"type": "file", "file_id": "file_synthetic_document"},
		{"type": "base64", "media_type": "application/octet-stream", "data": base64.StdEncoding.EncodeToString(make([]byte, 17))},
	} {
		t.Run(fmt.Sprint(source["type"]), func(t *testing.T) {
			document := claudePDFPageDocument("")
			document["source"] = source
			documents := []any{document}
			quote := requireClaudePDFPageQuote(t, claudePDFPageRequest(documents, false))
			require.Equal(t, claudePDFPageMetadataTokens(t, documents)+config.ClaudeNativeDocumentTokenAllowance, quote)
		})
	}
	documents := []any{claudePDFPageDocument(base64.StdEncoding.EncodeToString(make([]byte, 17)))}
	quote := requireClaudePDFPageQuote(t, claudePDFPageRequest(documents, false))
	require.Equal(t, claudePDFPageMetadataTokens(t, documents)+documentedPDFContextCeiling, quote,
		"bytes without page evidence reserve the documented request maximum")
}

// TestClaudeNativePDFPageRuntimeEstimate verifies that only the trusted
// operator setting changes the per-page estimate while metadata stays fixed.
func TestClaudeNativePDFPageRuntimeEstimate(t *testing.T) {
	original := config.ClaudeNativePDFTokensPerPage
	t.Cleanup(func() { config.ClaudeNativePDFTokensPerPage = original })
	documents := []any{claudePDFPageDocument(claudePDFPagePolicyFixture(t, 3, true))}
	request := claudePDFPageRequest(documents, false)
	metadata := claudePDFPageMetadataTokens(t, documents)
	config.ClaudeNativePDFTokensPerPage = 1000
	low := requireClaudePDFPageQuote(t, request)
	config.ClaudeNativePDFTokensPerPage = 2000
	high := requireClaudePDFPageQuote(t, request)
	require.Equal(t, metadata+3000, low)
	require.Equal(t, metadata+6000, high)
	config.ClaudeNativePDFTokensPerPage = 1048576
	require.Equal(t, metadata+documentedPDFContextCeiling, requireClaudePDFPageQuote(t, request))
}
