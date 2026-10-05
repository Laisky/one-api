package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// claudePDFAdversarialLine is one line of dense, tokenizer-diverse page text.
// Every page repeats it, so Flate compresses the whole document to a few KiB.
const claudePDFAdversarialLine = "Dense synthetic adversarial page text with many distinct words alpha beta gamma delta epsilon zeta eta theta iota kappa"

// claudePDFAdversarialLines draws about 2,700 tokenizer-proxy tokens of
// extractable text per page, inside the documented "typical" 1,500-3,000 range.
const claudePDFAdversarialLines = 60

// TestSecurityClaudeNativePDFPageQuoteFloor drives the production native quote
// with valid many-page PDFs (validated offline with poppler pdfinfo/pdftotext
// and pypdf) whose decoded size is tiny because every page shares one Flate
// content stream. The admission estimate must not fall below the documented
// per-page cost for the pages the document visibly declares.
func TestSecurityClaudeNativePDFPageQuoteFloor(t *testing.T) {
	pageText := strings.Repeat(claudePDFAdversarialLine+"\n", claudePDFAdversarialLines)
	pageTextTokens := openai.CountTokenText(pageText, claudePDFPageFixtureModel)
	for _, tc := range []struct {
		name    string
		options claudePDFPageFixtureOptions
	}{
		{name: "classic_xref_100", options: claudePDFPageFixtureOptions{Pages: 100}},
		{name: "object_stream_100", options: claudePDFPageFixtureOptions{Pages: 100, ObjectStream: true}},
		{name: "escaped_names_100", options: claudePDFPageFixtureOptions{Pages: 100, EscapedNames: true}},
		{name: "shared_kids_100", options: claudePDFPageFixtureOptions{Pages: 100, SharedKids: true}},
		{name: "object_stream_600", options: claudePDFPageFixtureOptions{Pages: 600, ObjectStream: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.options.LineText = claudePDFAdversarialLine
			tc.options.Lines = claudePDFAdversarialLines
			pdf := claudePDFPageFixture(t, tc.options)
			documents := []any{claudePDFPageDocument(base64.StdEncoding.EncodeToString(pdf))}
			request := claudePDFPageRequest(documents, false)
			quote, err := preparedClaudePromptTokens(context.Background(), request, nil)
			require.NoError(t, err)
			metadata := claudePDFPageMetadataTokens(t, documents)
			floor := metadata + documentedPDFPageQuote(tc.options.Pages)
			oldLinear := (len(pdf)*64 + 1023) / 1024
			t.Logf("PDF_PAGE_FLOOR case=%s decoded_bytes=%d pages=%d page_text_tokens=%d text_only_tokens=%d old_linear_64_per_kib=%d documented_floor=%d quote=%d",
				tc.name, len(pdf), tc.options.Pages, pageTextTokens, pageTextTokens*tc.options.Pages, oldLinear, floor, quote)
			require.GreaterOrEqual(t, quote, floor,
				"a %d-byte PDF declaring %d text-dense pages must reserve at least the documented per-page cost", len(pdf), tc.options.Pages)
		})
	}
}

// TestSecurityClaudeNativePDFPageQuoteNoGrossOverReserve keeps a valid
// one-page PDF whose page image is stored uncompressed (about 787 KB) within
// the documented one-page cost; decoded bytes of an embedded image do not add
// provider tokens beyond the rasterized page image.
func TestSecurityClaudeNativePDFPageQuoteNoGrossOverReserve(t *testing.T) {
	for _, compressed := range []bool{true, false} {
		data := claudeDocumentQuotePDF(t, compressed)
		documents := []any{claudePDFPageDocument(data)}
		quote, err := preparedClaudePromptTokens(context.Background(), claudePDFPageRequest(documents, false), nil)
		require.NoError(t, err)
		ceiling := claudePDFPageMetadataTokens(t, documents) + documentedPDFPageQuote(1)
		t.Logf("PDF_PAGE_SINGLE compressed=%v encoded_bytes=%d quote=%d one_page_documented=%d", compressed, len(data), quote, ceiling)
		require.LessOrEqual(t, quote, ceiling, "a one-page PDF must not reserve more than one documented page")
	}
}

// TestSecurityClaudeNativePDFPageAdmissionHTTP proves the compact 100-page PDF
// cannot be dispatched on a balance below its documented page cost, while
// funded requests forward untouched bytes and settle authoritative receipts.
func TestSecurityClaudeNativePDFPageAdmissionHTTP(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pages   int
		balance int64
		missing bool
		blocked bool
	}{
		{name: "compact_100_underfunded", pages: 100, balance: 200_000, blocked: true},
		{name: "compact_100_funded_measured", pages: 100, balance: 2_000_000},
		{name: "compact_100_funded_missing_usage", pages: 100, balance: 2_000_000, missing: true},
		{name: "compact_1_small_balance_measured", pages: 1, balance: 20_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, tc.balance, false)
			canonicalAdmissionConfiguration(t)
			pdf := base64.StdEncoding.EncodeToString(claudePDFPageFixture(t, claudePDFPageFixtureOptions{
				Pages: tc.pages, ObjectStream: true, LineText: claudePDFAdversarialLine, Lines: claudePDFAdversarialLines,
			}))
			document := claudePDFPageDocument(pdf)
			documents := []any{document}
			floor := int64(claudePDFPageMetadataTokens(t, documents) + documentedPDFPageQuote(tc.pages) + 8)
			if tc.blocked {
				require.Greater(t, floor, tc.balance, "balance must sit below the documented page cost")
			} else {
				require.Less(t, floor, tc.balance)
			}
			var calls atomic.Int32
			seen := make(chan []byte, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
				select {
				case seen <- body:
				default:
				}
				response := map[string]any{
					"id": "synthetic-page-doc", "type": "message", "role": "assistant", "model": claudePDFPageFixtureModel,
					"content": []any{map[string]any{"type": "text", "text": "ok"}}, "stop_reason": "end_turn",
				}
				if !tc.missing {
					response["usage"] = map[string]any{"input_tokens": 11, "output_tokens": 7}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			t.Cleanup(server.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = server.Client()
			client.HTTPClient.Timeout = 5 * time.Second
			t.Cleanup(func() { client.HTTPClient = oldClient })
			request := claudePDFPageRequest(documents, false)
			request.Model = "alias"
			request.MaxTokens = 8
			body, err := json.Marshal(request)
			require.NoError(t, err)
			c, _, id := protocolContext(t, channeltype.Anthropic, claudePDFPageFixtureModel, "/v1/messages", string(body), server.URL,
				tc.balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			apiErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			hold := c.GetInt64(ctxkey.PreConsumedQuotaAmount)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			var costRows int64
			require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costRows).Error)
			t.Logf("PDF_PAGE_HTTP case=%s encoded=%d balance=%d documented_floor=%d hold=%d calls=%d user_debit=%d cost_rows=%d",
				tc.name, len(pdf), tc.balance, floor, hold, calls.Load(), tc.balance-reloadUserQuota(t), costRows)
			if tc.blocked {
				require.Zero(t, calls.Load(), "an underfunded many-page PDF must not reach the provider")
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
				require.Equal(t, tc.balance, reloadUserQuota(t))
				require.Equal(t, tc.balance, token.RemainQuota)
				require.Zero(t, costRows)
				return
			}
			require.EqualValues(t, 1, calls.Load())
			var wire map[string]any
			require.NoError(t, json.Unmarshal(<-seen, &wire))
			outgoing := wire["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
			require.Equal(t, document, outgoing, "quoting must preserve the provider document bytes")
			require.GreaterOrEqual(t, hold, floor)
			wantDebit := int64(18)
			if tc.missing {
				require.NotNil(t, apiErr, "preserve the native incomplete-receipt response contract")
				wantDebit = hold
			} else {
				require.Nil(t, apiErr)
			}
			require.Equal(t, tc.balance-wantDebit, reloadUserQuota(t))
			require.Equal(t, tc.balance-wantDebit, token.RemainQuota)
			require.Equal(t, wantDebit, requestCostQuota(t, id))
			require.EqualValues(t, 1, costRows)
		})
	}
}
