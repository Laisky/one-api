package controller

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestClaudeNativePDFPageHTTP tests encoding-independent admission of the same
// rendered page, original PDF forwarding, authoritative usage, and retained
// estimate settlement when the receipt is missing.
func TestClaudeNativePDFPageHTTP(t *testing.T) {
	for _, tc := range []struct {
		name       string
		compressed bool
		balance    int64
		missing    bool
		blocked    bool
	}{
		{name: "compact_measured", compressed: true, balance: 40000},
		{name: "raw_same_balance_measured", balance: 40000},
		{name: "compact_missing_usage", compressed: true, balance: 40000, missing: true},
		{name: "raw_missing_usage", balance: 40000, missing: true},
		{name: "compact_underfunded", compressed: true, balance: 1, blocked: true},
		{name: "raw_underfunded", balance: 8000, blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, tc.balance, false)
			canonicalAdmissionConfiguration(t)
			pdf := claudeDocumentQuotePDF(t, tc.compressed)
			decoded, err := base64.StdEncoding.DecodeString(pdf)
			require.NoError(t, err)
			document := claudePDFPageDocument(pdf)
			documents := []any{document}
			wantHold := int64(claudePDFPageMetadataTokens(t, documents) + documentedPDFPageQuote(1) + 8)
			if tc.blocked {
				require.Greater(t, wantHold, tc.balance, "balance must be below the documented one-page quote")
			} else {
				require.Less(t, wantHold, tc.balance)
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
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
			var costRows int64
			require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&costRows).Error)
			t.Logf("PDF_PAGE_ONE_HTTP case=%s decoded=%d balance=%d policy_hold=%d actual_hold=%d calls=%d user_debit=%d cost_rows=%d",
				tc.name, len(decoded), tc.balance, wantHold, c.GetInt64(ctxkey.PreConsumedQuotaAmount), calls.Load(),
				tc.balance-reloadUserQuota(t), costRows)
			if tc.blocked {
				require.Zero(t, calls.Load(), "policy-underfunded request must not reach the provider")
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
				require.Equal(t, tc.balance, reloadUserQuota(t))
				require.Equal(t, tc.balance, token.RemainQuota)
				require.Zero(t, costRows)
				require.Empty(t, logs)
				return
			}
			require.EqualValues(t, 1, calls.Load())
			var wire map[string]any
			require.NoError(t, json.Unmarshal(<-seen, &wire))
			require.Equal(t, claudePDFPageFixtureModel, wire["model"])
			require.EqualValues(t, 8, wire["max_tokens"])
			outgoing := wire["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)
			require.Equal(t, document, outgoing, "private quote projection must preserve the provider document")
			require.Equal(t, wantHold, c.GetInt64(ctxkey.PreConsumedQuotaAmount), "raw and compressed encodings of one page hold the same quote")
			wantDebit := int64(18)
			if tc.missing {
				require.NotNil(t, apiErr, "preserve the native incomplete-receipt response contract")
				wantDebit = wantHold
			} else {
				require.Nil(t, apiErr)
			}
			require.Equal(t, tc.balance-wantDebit, reloadUserQuota(t))
			require.Equal(t, tc.balance-wantDebit, token.RemainQuota)
			require.Equal(t, wantDebit, requestCostQuota(t, id))
			require.EqualValues(t, 1, costRows)
			require.Len(t, logs, 1)
			require.Equal(t, model.LogTypeConsume, logs[0].Type)
			require.EqualValues(t, wantDebit, logs[0].Quota)
			if tc.missing {
				require.Equal(t, true, logs[0].Metadata["billing_estimated"])
				require.NotEmpty(t, logs[0].Metadata["billing_estimate_reason"])
			} else {
				require.Empty(t, logs[0].Metadata["billing_estimate_reason"], "complete measured usage remains authoritative")
			}
		})
	}
}
