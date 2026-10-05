package controller

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestClaudeNativePDFPageValidationHTTP rejects invalid sources, unrepresentable
// priced quotes and context-ceiling underfunding before TLS dispatch or durable
// accounting changes.
func TestClaudeNativePDFPageValidationHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, errorCode string
		data            any
		badData         bool
		perPage         int
		tariff          float64
		status          int
	}{
		{name: "malformed", data: "not-base64!", badData: true, perPage: 8684, tariff: 1, status: 400, errorCode: "invalid_claude_prompt_quote"},
		{name: "empty", data: "", badData: true, perPage: 8684, tariff: 1, status: 400, errorCode: "invalid_claude_prompt_quote"},
		{name: "non_string", data: 17, badData: true, perPage: 8684, tariff: 1, status: 400, errorCode: "invalid_claude_prompt_quote"},
		{name: "monetary_overflow", perPage: 8684, tariff: 1e16, status: 400, errorCode: "invalid_claude_quota_quote"},
		{name: "context_ceiling_underfunded", perPage: 1048576, tariff: 1, status: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100000)
			xaiVideoSetup(t, balance, false)
			canonicalAdmissionConfiguration(t)
			require.Equal(t, "sqlite", model.DB.Dialector.Name())
			oldPerPage := config.ClaudeNativePDFTokensPerPage
			config.ClaudeNativePDFTokensPerPage = tc.perPage
			t.Cleanup(func() { config.ClaudeNativePDFTokensPerPage = oldPerPage })
			pdf := claudeDocumentQuotePDF(t, true)
			document := claudePDFPageDocument(pdf)
			if tc.badData {
				document["source"].(map[string]any)["data"] = tc.data
			}
			documents := []any{document}
			request := claudePDFPageRequest(documents, false)
			request.MaxTokens = 8
			quote, quoteErr := preparedClaudePromptTokens(context.Background(), request, nil)
			if tc.badData {
				require.Error(t, quoteErr)
			} else {
				require.NoError(t, quoteErr)
				expected := claudePDFPageMetadataTokens(t, documents) + min(tc.perPage, documentedPDFContextCeiling)
				require.Equal(t, expected, quote, "one page at the configured estimate, capped at the context ceiling")
				if tc.name == "monetary_overflow" {
					budget := new(big.Float).Mul(big.NewFloat(float64(quote+8)), big.NewFloat(tc.tariff))
					require.Positive(t, budget.Cmp(big.NewFloat(math.MaxInt64)), "only monetary conversion should overflow in this case")
				}
			}
			var beforeUser model.User
			var beforeToken model.Token
			require.NoError(t, model.DB.First(&beforeUser, fallbackUserID).Error)
			require.NoError(t, model.DB.First(&beforeToken, fallbackTokenID).Error)
			var calls atomic.Int32
			seen := make(chan []byte, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "synthetic-validation", "type": "message", "role": "assistant", "model": claudePDFPageFixtureModel,
					"content": []any{map[string]any{"type": "text", "text": "ok"}}, "stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 11, "output_tokens": 7}})
				select {
				case seen <- body:
				default:
				}
			}))
			t.Cleanup(server.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = server.Client()
			client.HTTPClient.Timeout = 5 * time.Second
			t.Cleanup(func() { client.HTTPClient = oldClient })
			request.Model = "alias"
			body, err := json.Marshal(request)
			require.NoError(t, err)
			c, _, id := protocolContext(t, channeltype.Anthropic, claudePDFPageFixtureModel, "/v1/messages", string(body), server.URL,
				balance, 1, false, &model.ModelConfigLocal{Ratio: tc.tariff, CompletionRatio: 1})
			apiErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			t.Logf("PDF_PAGE_VALIDATION case=%s per_page=%d prompt_quote=%d calls=%d held=%d", tc.name, tc.perPage, quote, calls.Load(), c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			if calls.Load() > 0 {
				t.Logf("unexpected provider dispatch captured_bytes=%d", len(<-seen))
			}
			require.Zero(t, calls.Load())
			require.NotNil(t, apiErr)
			require.Equal(t, tc.status, apiErr.StatusCode)
			if tc.errorCode != "" {
				require.Equal(t, "one_api_error", string(apiErr.Type))
				require.Equal(t, tc.errorCode, apiErr.Code)
			}
			require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			var afterUser model.User
			var afterToken model.Token
			require.NoError(t, model.DB.First(&afterUser, fallbackUserID).Error)
			require.NoError(t, model.DB.First(&afterToken, fallbackTokenID).Error)
			require.Equal(t, beforeUser.Quota, afterUser.Quota)
			require.Equal(t, beforeUser.UsedQuota, afterUser.UsedQuota)
			require.Equal(t, beforeToken.RemainQuota, afterToken.RemainQuota)
			require.Equal(t, beforeToken.UsedQuota, afterToken.UsedQuota)
			for _, table := range []any{&model.UserRequestCost{}, &model.QuotaRefund{}} {
				var rows int64
				require.NoError(t, model.DB.Model(table).Where("request_id = ?", id).Count(&rows).Error)
				require.Zero(t, rows)
			}
			var logRows int64
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", id).Count(&logRows).Error)
			require.Zero(t, logRows)
		})
	}
}
