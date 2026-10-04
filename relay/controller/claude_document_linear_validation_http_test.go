package controller

import (
	"context"
	"encoding/base64"
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

// TestClaudeNativePDFLinearValidationHTTP rejects invalid source and arithmetic quotes before TLS dispatch or durable accounting changes.
func TestClaudeNativePDFLinearValidationHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, errorCode string
		data            any
		badData         bool
		rate            int
		tariff          float64
		status          int
	}{
		{name: "malformed", data: "not-base64!", badData: true, rate: 64, tariff: 1, status: 400, errorCode: "invalid_claude_prompt_quote"},
		{name: "empty", data: "", badData: true, rate: 64, tariff: 1, status: 400, errorCode: "invalid_claude_prompt_quote"},
		{name: "non_string", data: 17, badData: true, rate: 64, tariff: 1, status: 400, errorCode: "invalid_claude_prompt_quote"},
		{name: "prompt_overflow", rate: math.MaxInt, tariff: 1, status: 400, errorCode: "invalid_claude_prompt_quote"},
		{name: "monetary_overflow", rate: math.MaxInt / 16, tariff: 32, status: 400, errorCode: "invalid_claude_quota_quote"},
		{name: "uncapped_positive_rate", rate: 1048577, tariff: 1, status: 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100000)
			xaiVideoSetup(t, balance, false)
			canonicalAdmissionConfiguration(t)
			require.Equal(t, "sqlite", model.DB.Dialector.Name())
			oldRate := config.ClaudeNativePDFTokensPerKiB
			config.ClaudeNativePDFTokensPerKiB = tc.rate
			t.Cleanup(func() { config.ClaudeNativePDFTokensPerKiB = oldRate })
			pdf := claudeDocumentQuotePDF(t, true)
			decoded, err := base64.StdEncoding.DecodeString(pdf)
			require.NoError(t, err)
			document := linearPDFReviewDocument(pdf)
			if tc.badData {
				document["source"].(map[string]any)["data"] = tc.data
			}
			documents := []any{document}
			request := linearPDFReviewRequest(documents, false)
			request.MaxTokens = 8
			quote, quoteErr := preparedClaudePromptTokens(context.Background(), request, nil)
			if tc.badData || tc.rate == math.MaxInt {
				require.Error(t, quoteErr)
			} else {
				require.NoError(t, quoteErr)
				expected := new(big.Int).Mul(big.NewInt(int64(len(decoded))), big.NewInt(int64(tc.rate)))
				expected.Add(expected, big.NewInt(1023)).Quo(expected, big.NewInt(1024))
				expected.Add(expected, big.NewInt(int64(linearPDFReviewMetadataTokens(t, documents))))
				require.True(t, expected.IsInt64(), "the native prompt quote itself must be representable")
				require.EqualValues(t, expected.Int64(), quote)
				if tc.name == "monetary_overflow" {
					budget := new(big.Int).Add(expected, big.NewInt(8))
					budget.Mul(budget, big.NewInt(32))
					require.False(t, budget.IsInt64(), "only monetary conversion should overflow in this case")
				} else {
					require.Greater(t, quote, 1048576, "positive rate must not clamp the resulting estimate")
				}
			}
			var beforeUser model.User
			var beforeToken model.Token
			require.NoError(t, model.DB.First(&beforeUser, fallbackUserID).Error)
			require.NoError(t, model.DB.First(&beforeToken, fallbackTokenID).Error)
			var calls atomic.Int32
			seen := make(chan linearPDFHTTPObservation, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, readErr := io.ReadAll(io.LimitReader(r.Body, 64<<10))
				w.Header().Set("Content-Type", "application/json")
				writeErr := json.NewEncoder(w).Encode(map[string]any{"id": "synthetic-validation", "type": "message", "role": "assistant", "model": linearPDFReviewModel,
					"content": []any{map[string]any{"type": "text", "text": "ok"}}, "stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 11, "output_tokens": 7}})
				seen <- linearPDFHTTPObservation{path: r.URL.Path, body: body, readErr: readErr, writeErr: writeErr}
			}))
			t.Cleanup(server.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = server.Client()
			client.HTTPClient.Timeout = 5 * time.Second
			t.Cleanup(func() { client.HTTPClient = oldClient })
			request.Model = "alias"
			body, err := json.Marshal(request)
			require.NoError(t, err)
			c, _, id := protocolContext(t, channeltype.Anthropic, linearPDFReviewModel, "/v1/messages", string(body), server.URL,
				balance, 1, false, &model.ModelConfigLocal{Ratio: tc.tariff, CompletionRatio: 1})
			apiErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			t.Logf("LINEAR_PDF_VALIDATION case=%s rate=%d prompt_quote=%d calls=%d held=%d", tc.name, tc.rate, quote, calls.Load(), c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			if calls.Load() > 0 {
				observed := <-seen
				require.NoError(t, observed.readErr)
				require.NoError(t, observed.writeErr)
				t.Logf("unexpected provider dispatch path=%s captured_bytes=%d", observed.path, len(observed.body))
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

// TestClaudeNativePDFLinearRuntimeRate verifies that a trusted runtime rate changes the decoded-byte quote and leaves metadata fixed.
func TestClaudeNativePDFLinearRuntimeRate(t *testing.T) {
	oldRate := config.ClaudeNativePDFTokensPerKiB
	t.Cleanup(func() { config.ClaudeNativePDFTokensPerKiB = oldRate })
	data := base64.StdEncoding.EncodeToString(make([]byte, 17))
	documents := []any{linearPDFReviewDocument(data)}
	request := linearPDFReviewRequest(documents, false)
	metadata := linearPDFReviewMetadataTokens(t, documents)
	config.ClaudeNativePDFTokensPerKiB = 64
	low, err := preparedClaudePromptTokens(context.Background(), request, nil)
	require.NoError(t, err)
	config.ClaudeNativePDFTokensPerKiB = 128
	high, err := preparedClaudePromptTokens(context.Background(), request, nil)
	require.NoError(t, err)
	require.Equal(t, metadata+2, low)
	require.Equal(t, metadata+3, high)
	require.Equal(t, 1, high-low)
}
