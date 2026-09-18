package controller

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
)

// TestJinaBillingCompletedStreamSettlesBeyondBalance verifies that already
// performed work reaches debt-capable final settlement instead of being rejected
// by the generic streaming tracker's admission check. Both finite and unlimited
// tokens still debit the user, and channel overrides use the same final receipt.
func TestJinaBillingCompletedStreamSettlesBeyondBalance(t *testing.T) {
	const wire = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1000000,\"completion_tokens\":1000000,\"total_tokens\":2000000}}\n\ndata: [DONE]\n\n"
	for _, unlimited := range []bool{false, true} {
		for _, override := range []float64{-1, 1} {
			t.Run(fmt.Sprintf("unlimited=%v/override=%v", unlimited, override), func(t *testing.T) {
				const balance = int64(1000000)
				transport := &jinaReceiptFaultTransport{wire: wire, contentType: "text/event-stream", terminal: io.EOF}
				previous := client.HTTPClient
				client.HTTPClient = &http.Client{Transport: transport}
				defer func() { client.HTTPClient = previous }()
				c, requestID := jinaAuditContext(t, "/v1/chat/completions",
					`{"model":"alias","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"read this image"}]}`,
					"https://api.jina.ai", "jina-ocr-v1", balance, unlimited, 1, override)
				t.Cleanup(func() { drainBilling(t) })
				apiErr := RelayTextHelper(c)
				drainBilling(t)
				require.Nil(t, apiErr, "a complete paid receipt must not be rejected by an admission check after inference")
				require.True(t, c.GetBool(ctxkey.BillingReconciled))
				charge := int64(1250000)
				if override >= 0 {
					charge = 5000000
				}
				require.Greater(t, charge, balance)
				jinaAuditAssertLedger(t, requestID, balance, charge, unlimited, false)
				require.EqualValues(t, 1, transport.calls.Load())
				require.EqualValues(t, 1, transport.closes.Load())
			})
		}
	}
}
