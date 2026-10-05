package controller

import (
	"net/http"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/streaming"
)

// finalizeStreamingUsage separates future-spending enforcement from settlement.
// It always returns observed usage and successful incremental debits, including
// when the last delta could not be funded. Existing callers own one final debit.
func finalizeStreamingUsage(tracker *streaming.QuotaTracker, usage *relaymodel.Usage, responseErr *relaymodel.ErrorWithStatusCode) (*relaymodel.Usage, int64, *relaymodel.ErrorWithStatusCode) {
	if tracker == nil {
		return usage, 0, responseErr
	}
	observed, charged, err := tracker.Finalize(usage)
	if err != nil && responseErr == nil {
		if errors.Is(err, streaming.ErrQuotaExceeded) {
			responseErr = openai.ErrorWrapper(err, "insufficient_user_quota", http.StatusForbidden)
		} else {
			responseErr = openai.ErrorWrapper(err, "streaming_billing_failed", http.StatusInternalServerError)
		}
	}
	return observed, charged, responseErr
}
