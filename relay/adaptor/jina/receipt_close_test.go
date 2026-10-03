package jina

import (
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// receiptCloseAuditBody records physical transport closure without changing reads.
type receiptCloseAuditBody struct {
	io.Reader
	closes atomic.Int32
	err    error
}

// Close counts calls so concurrent or repeated wrapper closure can be verified.
func (b *receiptCloseAuditBody) Close() error {
	b.closes.Add(1)
	return b.err
}

// TestOCRReceiptCloseOnce checks cleanup after both successful and failed close,
// including concurrent callers and preservation of the original failure cause.
func TestOCRReceiptCloseOnce(t *testing.T) {
	t.Parallel()
	for _, closeErr := range []error{nil, io.ErrClosedPipe} {
		body := &receiptCloseAuditBody{Reader: strings.NewReader(auditOCRReceipt), err: closeErr}
		observer := &receiptBody{ReadCloser: body}
		_, err := io.ReadAll(observer)
		require.NoError(t, err)
		const callers = 20
		results := make(chan error, callers)
		var workers sync.WaitGroup
		for range callers {
			workers.Add(1)
			go func() {
				defer workers.Done()
				results <- observer.Close()
			}()
		}
		workers.Wait()
		close(results)
		for err := range results {
			if closeErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, closeErr)
			}
		}
		require.EqualValues(t, 1, body.closes.Load())
		usage, invalid := observer.snapshot()
		require.NotNil(t, usage)
		require.Equal(t, closeErr != nil, invalid)
	}
}
