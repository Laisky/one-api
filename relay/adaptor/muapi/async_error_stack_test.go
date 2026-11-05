package muapi

import (
	stderrors "errors"
	"fmt"
	"testing"

	stackerrors "github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestMismatchedAsyncReceiptHasReturnSiteStack verifies that a mismatched task identity keeps its cause and excludes provider details while recording its source.
func TestMismatchedAsyncReceiptHasReturnSiteStack(t *testing.T) {
	for _, body := range []string{`{"id":"another-job","status":"failed"}`, `{"request_id":"another-job","status":"failed"}`} {
		_, err := normalizeAsyncObservation([]byte(body), "paid-job")
		require.ErrorIs(t, err, errMuAPIResultTaskIDMismatch)
		require.EqualError(t, err, "MuAPI result task ID mismatch")
		var trace interface{ StackTrace() stackerrors.StackTrace }
		require.True(t, stderrors.As(err, &trace))
		require.Contains(t, fmt.Sprintf("%+v", trace.StackTrace()), "normalizeAsyncObservation")
	}
}
