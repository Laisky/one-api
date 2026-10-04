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
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const linearPDFReviewModel = "claude-sonnet-5-5"

// linearPDFReviewUnits returns the review calibration of 64 tokens per KiB for bounded decoded byte counts.
// This operator estimate is not a claim about provider usage or a PDF parser.
func linearPDFReviewUnits(decodedBytes int) int {
	return (decodedBytes + 15) / 16
}

// linearPDFReviewDocument returns a base64 document with known metadata for a supplied transport payload.
func linearPDFReviewDocument(data string) map[string]any {
	return map[string]any{
		"type": "document", "title": "Synthetic PDF", "context": "Read this fixture.",
		"source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": data},
	}
}

// linearPDFReviewRequest returns a one-message native request with documents directly or inside a tool result.
func linearPDFReviewRequest(documents []any, nested bool) *ClaudeMessagesRequest {
	content := documents
	if nested {
		content = []any{map[string]any{"type": "tool_result", "tool_use_id": "synthetic-call", "content": documents}}
	}
	return &ClaudeMessagesRequest{Model: linearPDFReviewModel, MaxTokens: 1,
		Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: content}}}
}

// linearPDFReviewMetadataTokens counts a manually specified provider-visible metadata view with the independent text tokenizer.
// It never calls the document projection or native document allowance helper under test.
func linearPDFReviewMetadataTokens(t *testing.T, documents []any) int {
	t.Helper()
	parts := make([]relaymodel.MessageContent, 0, len(documents))
	for _, value := range documents {
		document := value.(map[string]any)
		source := document["source"].(map[string]any)
		metadata := map[string]any{"type": "document", "source": map[string]any{"type": source["type"]}}
		if mediaType, present := source["media_type"]; present {
			metadata["source"].(map[string]any)["media_type"] = mediaType
		}
		for _, key := range []string{"title", "context", "citations"} {
			if value, present := document[key]; present {
				metadata[key] = value
			}
		}
		encoded, err := json.Marshal(metadata)
		require.NoError(t, err)
		text := string(encoded)
		parts = append(parts, relaymodel.MessageContent{Type: "text", Text: &text})
	}
	return openai.CountTokenMessages(context.Background(), []relaymodel.Message{{Role: "user", Content: parts}}, linearPDFReviewModel)
}

// TestClaudeNativePDFLinearSizePolicy exercises decoded-byte rounding and growth through the production prepared native quote.
// Zero-filled payloads are base64 encoding fixtures only; they are not asserted to be valid PDFs.
func TestClaudeNativePDFLinearSizePolicy(t *testing.T) {
	for _, size := range []int{1, 15, 16, 17, 1024, 65536, 65537, 131072, 1 << 20, 2 << 20} {
		t.Run(fmt.Sprintf("decoded_bytes=%d", size), func(t *testing.T) {
			data := base64.StdEncoding.EncodeToString(make([]byte, size))
			documents := []any{linearPDFReviewDocument(data)}
			request := linearPDFReviewRequest(documents, false)
			before, err := json.Marshal(request)
			require.NoError(t, err)
			quote, err := preparedClaudePromptTokens(context.Background(), request, nil)
			require.NoError(t, err)
			after, err := json.Marshal(request)
			require.NoError(t, err)
			require.Equal(t, before, after, "quoting must preserve transport bytes and caller ownership")
			metadata := linearPDFReviewMetadataTokens(t, documents)
			want := metadata + linearPDFReviewUnits(size)
			t.Logf("LINEAR_PDF_SIZE decoded=%d encoded=%d metadata=%d quote=%d review_quote=%d", size, len(data), metadata, quote, want)
			require.Equal(t, want, quote, "decoded-byte policy must round upward and continue growing without a token cap")
		})
	}
}

// TestClaudeNativePDFLinearStructure preserves exact metadata charges, per-document rounding, and nested result payloads.
func TestClaudeNativePDFLinearStructure(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, metadata := range []bool{false, true} {
			t.Run(fmt.Sprintf("nested=%v/metadata=%v", nested, metadata), func(t *testing.T) {
				documents := []any{
					linearPDFReviewDocument(base64.StdEncoding.EncodeToString(make([]byte, 17))),
					linearPDFReviewDocument(base64.StdEncoding.EncodeToString(make([]byte, 65537))),
				}
				if metadata {
					documents[0].(map[string]any)["title"] = strings.Repeat("Synthetic title ", 40)
					documents[1].(map[string]any)["context"] = strings.Repeat("Synthetic context ", 60)
					documents[1].(map[string]any)["citations"] = map[string]any{"enabled": true}
				}
				request := linearPDFReviewRequest(documents, nested)
				before, err := json.Marshal(request)
				require.NoError(t, err)
				quote, err := preparedClaudePromptTokens(context.Background(), request, nil)
				require.NoError(t, err)
				after, err := json.Marshal(request)
				require.NoError(t, err)
				require.Equal(t, before, after)
				want := linearPDFReviewMetadataTokens(t, documents) + linearPDFReviewUnits(17) + linearPDFReviewUnits(65537)
				t.Logf("LINEAR_PDF_STRUCTURE nested=%v metadata=%v quote=%d review_quote=%d", nested, metadata, quote, want)
				require.Equal(t, want, quote, "each document rounds separately and retains its metadata charge")
			})
		}
	}
}

// TestClaudeNativePDFLinearBase64Policy retains quote-view whitespace handling and rejects malformed or empty PDF source data.
func TestClaudeNativePDFLinearBase64Policy(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString(make([]byte, 17))
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
			document := linearPDFReviewDocument("")
			document["source"].(map[string]any)["data"] = tc.data
			documents := []any{document}
			quote, err := preparedClaudePromptTokens(context.Background(), linearPDFReviewRequest(documents, false), nil)
			t.Logf("LINEAR_PDF_BASE64 case=%s quote=%d error=%v", tc.name, quote, err)
			if tc.bad {
				require.Error(t, err, "invalid source must fail before paid dispatch rather than reserve zero or a flat allowance")
				return
			}
			require.NoError(t, err)
			require.Equal(t, linearPDFReviewMetadataTokens(t, documents)+linearPDFReviewUnits(17), quote)
		})
	}
}

// TestClaudeNativePDFLinearOpaqueControls preserves the separate URL, file, and non-PDF source allowance without remote fetching.
func TestClaudeNativePDFLinearOpaqueControls(t *testing.T) {
	for _, source := range []map[string]any{
		{"type": "url", "url": "https://example.invalid/synthetic.pdf"},
		{"type": "file", "file_id": "file_synthetic_document"},
		{"type": "base64", "media_type": "application/octet-stream", "data": base64.StdEncoding.EncodeToString(make([]byte, 17))},
	} {
		t.Run(fmt.Sprint(source["type"]), func(t *testing.T) {
			document := linearPDFReviewDocument("")
			document["source"] = source
			documents := []any{document}
			quote, err := preparedClaudePromptTokens(context.Background(), linearPDFReviewRequest(documents, false), nil)
			require.NoError(t, err)
			require.Equal(t, linearPDFReviewMetadataTokens(t, documents)+config.ClaudeNativeDocumentTokenAllowance, quote)
		})
	}
}
