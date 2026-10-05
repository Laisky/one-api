package muapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
)

// TestAsyncBillingHTTPErrorCannotAuthorizeRefund checks every post-dispatch HTTP
// error: status alone is not authoritative evidence of non-acceptance.
func TestAsyncBillingHTTPErrorCannotAuthorizeRefund(t *testing.T) {
	for _, code := range []int{400, 401, 402, 403, 404, 408, 409, 413, 422, 429, 500, 502, 503, 504} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				_, _ = io.WriteString(w, `{"error":"response failed after possible acceptance"}`)
			}))
			defer server.Close()
			receipt, err := (&Adaptor{}).SubmitVideo(context.Background(), &meta.Meta{BaseURL: server.URL, ActualModelName: "veo3-fast", APIKey: "fixture"}, []byte(`{"duration":5}`))
			require.Error(t, err)
			require.False(t, receipt.Rejected, "HTTP %d cannot prove paid work was never accepted", code)
		})
	}
}

// TestAsyncBillingReceiptSurvivesHTTPError preserves a valid paid-job receipt
// even when a gateway supplied an erroneous non-2xx status.
func TestAsyncBillingReceiptSurvivesHTTPError(t *testing.T) {
	for _, code := range []int{400, 409, 429, 500, 502} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
				_, _ = io.WriteString(w, `{"request_id":"accepted-paid-job","cost":{"amount_usd":0.4}}`)
			}))
			defer server.Close()
			receipt, _ := (&Adaptor{}).SubmitVideo(context.Background(), &meta.Meta{BaseURL: server.URL, ActualModelName: "veo3-fast"}, []byte(`{"duration":5}`))
			require.Equal(t, "accepted-paid-job", receipt.ID)
			require.False(t, receipt.Rejected)
		})
	}
}

// TestAsyncBillingRefundNeedsMatchingReceipt disallows release of another job's
// hold from an anonymous or mismatched refund observation.
func TestAsyncBillingRefundNeedsMatchingReceipt(t *testing.T) {
	for _, body := range []string{`{"status":"failed","cost":{"refunded":true}}`, `{"id":"another-job","status":"failed","cost":{"refunded":true}}`} {
		_, err := normalizeAsyncObservation([]byte(body), "paid-job")
		require.Error(t, err)
	}
}
