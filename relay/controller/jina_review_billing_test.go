package controller

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/jina"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestJinaOversizedReceiptLedger verifies out-of-range counters do not pass as
// measured usage or enter unchecked float settlement. Higher evidence is charged
// conservatively with an estimate marker, not dropped to a small reservation.
func TestJinaOversizedReceiptLedger(t *testing.T) {
	for _, path := range []string{"/v1/embeddings", "/v1/rerank"} {
		for _, tc := range []struct {
			name, counter string
			charge        int64
		}{
			{"above_admission_limit", "67108865", 1677722},
			{"int64_limit", "9223372036854775807", jina.MaxReceiptQuota},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				const balance = int64(100000000)
				field, actual := "data", "jina-embeddings-v3"
				body := `{"model":"alias","input":["hello"]}`
				if path == "/v1/rerank" {
					field, actual = "results", "jina-reranker-v3.5"
					body = `{"model":"alias","query":"q","documents":["d"]}`
				}
				wire := `{"` + field + `":[],"usage":{"total_tokens":` + tc.counter + `}}`
				transport := &jinaReceiptFaultTransport{wire: wire, contentType: "application/json", terminal: io.EOF}
				previous := client.HTTPClient
				client.HTTPClient = &http.Client{Transport: transport}
				defer func() { client.HTTPClient = previous }()
				c, requestID := jinaAuditContext(t, path, body, "https://api.jina.ai", actual, balance, false, 1, -1)
				t.Cleanup(func() { drainBilling(t) })
				var apiErr *relaymodel.ErrorWithStatusCode
				if path == "/v1/rerank" {
					apiErr = RelayRerankHelper(c)
				} else {
					apiErr = RelayTextHelper(c)
				}
				drainBilling(t)
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
				require.Greater(t, tc.charge, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
				jinaAuditAssertLedger(t, requestID, balance, tc.charge, false, true)
				require.False(t, BillingAllowsRetry(c))
				require.EqualValues(t, 1, transport.calls.Load())
				require.EqualValues(t, 1, transport.closes.Load())
			})
		}
	}
}

// TestJinaFinalQuotaNeverUsesUncheckedFallback injects extreme shared-calculator
// results and proves the Jina settlement result is independent of those floats.
func TestJinaFinalQuotaNeverUsesUncheckedFallback(t *testing.T) {
	for _, calculated := range []int64{math.MinInt64, 0, math.MaxInt64} {
		t.Run(fmt.Sprint(calculated), func(t *testing.T) {
			usage := &relaymodel.Usage{PromptTokens: jina.MaxBillingTokens + 1}
			amount := exactJinaUsageQuota(context.Background(), &meta.Meta{ChannelType: channeltype.Jina}, usage,
				calculated, 100, .025, 0, 1)
			require.EqualValues(t, 1677722, amount)
			require.NotEmpty(t, usage.BillingEstimateReason)
			usage = &relaymodel.Usage{PromptTokens: math.MaxInt, CompletionTokens: math.MaxInt}
			amount = exactJinaUsageQuota(context.Background(), &meta.Meta{ChannelType: channeltype.Jina}, usage,
				calculated, 100, 1, 4, 1)
			require.Equal(t, jina.MaxReceiptQuota, amount)
			require.Contains(t, usage.BillingEstimateReason, "reconciliation")
		})
	}
}
