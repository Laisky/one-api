package controller

import (
	"bytes"
	"io"
	"net/http"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// restoredResponseBody replays an inspected prefix before the unread remainder
// while closing the original provider body.
type restoredResponseBody struct {
	io.Reader
	closer io.Closer
}

// Close closes the original provider body. It takes no parameters and returns
// the underlying close error.
func (b restoredResponseBody) Close() error { return b.closer.Close() }

// paidUpstreamErrorUsage inspects a non-2xx provider response for a receipt of
// billable work before the caller refunds or replays the attempt. It reads at
// most adaptor.MaxPaidErrorReceiptBytes, restores the body for unchanged error
// rendering, and asks the adaptor's provider-specific contract. Parameters: c
// carries the request logger, m is the attempt metadata, ad is the serving
// adaptor, and resp is the provider response. Returns: the usage to settle, or
// nil when the provider does not bill this error or the body carries no receipt.
func paidUpstreamErrorUsage(c *gin.Context, m *metalib.Meta, ad adaptor.Adaptor, resp *http.Response) *relaymodel.Usage {
	reader, ok := ad.(adaptor.PaidErrorReceiptReader)
	if !ok || m == nil || resp == nil || resp.Body == nil {
		return nil
	}
	lg := gmw.GetLogger(c)
	prefix, err := io.ReadAll(io.LimitReader(resp.Body, adaptor.MaxPaidErrorReceiptBytes+1))
	resp.Body = restoredResponseBody{Reader: io.MultiReader(bytes.NewReader(prefix), resp.Body), closer: resp.Body}
	if err != nil {
		lg.Debug("paid upstream error receipt not inspected: body read failed",
			zap.Error(err), zap.Int("status_code", resp.StatusCode))
		return nil
	}
	if len(prefix) > adaptor.MaxPaidErrorReceiptBytes {
		lg.Debug("paid upstream error receipt not inspected: body exceeds bound",
			zap.Int("status_code", resp.StatusCode), zap.Int("limit_bytes", adaptor.MaxPaidErrorReceiptBytes))
		return nil
	}
	usage := reader.PaidErrorReceipt(c, m, resp.StatusCode, prefix)
	if usage == nil {
		return nil
	}
	lg.Info("upstream error response carries a usage receipt; settling the attempt instead of refunding it",
		zap.Int("status_code", resp.StatusCode),
		zap.String("model", m.ActualModelName),
		zap.Int("prompt_tokens", usage.PromptTokens),
		zap.Int("completion_tokens", usage.CompletionTokens),
		zap.String("billing_estimate_reason", usage.BillingEstimateReason),
	)
	return usage
}
