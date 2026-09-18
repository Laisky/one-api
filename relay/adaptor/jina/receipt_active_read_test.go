package jina

import (
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// gatedReceiptReader exposes a complete-looking receipt, then parks an active
// read until the test releases it. Close intentionally does not join the read,
// reproducing the shared heartbeat reader's cancellation lifecycle.
type gatedReceiptReader struct {
	first   *strings.Reader
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

// Read delivers the first receipt or waits and returns later bytes with an error.
func (r *gatedReceiptReader) Read(dst []byte) (int, error) {
	if r.first.Len() > 0 {
		return r.first.Read(dst)
	}
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return copy(dst, "data: {\"usage\":{\"prompt_tokens\":999,\"completion_tokens\":200,\"total_tokens\":1199}}\n\n"), io.ErrUnexpectedEOF
}

// Close models a transport that returns before an already-active Read unwinds.
func (r *gatedReceiptReader) Close() error { return nil }

// TestOCRReceiptCannotCertifyAnActiveRead deterministically prevents an early
// positive receipt and DONE from releasing the reservation while later bytes or
// errors remain unobserved. It does not rely on sleeps or scheduler luck.
func TestOCRReceiptCannotCertifyAnActiveRead(t *testing.T) {
	t.Parallel()
	r := &gatedReceiptReader{
		first:   strings.NewReader("data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n\ndata: [DONE]\n\n"),
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	observer := &receiptBody{ReadCloser: r, stream: true}
	_, err := observer.Read(make([]byte, 1024))
	require.NoError(t, err)
	_, invalid := observer.snapshot()
	require.False(t, invalid, "control: the complete receipt is certifiable without another read")
	finished := make(chan error, 1)
	go func() {
		_, err := observer.Read(make([]byte, 1024))
		finished <- err
	}()
	<-r.entered
	defer func() {
		close(r.release)
		require.ErrorIs(t, <-finished, io.ErrUnexpectedEOF)
		usage, invalid := observer.snapshot()
		require.True(t, invalid)
		require.Equal(t, 999, usage.PromptTokens, "late positive evidence must still be retained")
	}()
	_, invalid = observer.snapshot()
	require.True(t, invalid, "an active read cannot certify the earlier receipt")
	require.NoError(t, observer.Close())
	_, invalid = observer.snapshot()
	require.True(t, invalid, "Close must not certify a still-active read")
	_, err = observer.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.ErrClosedPipe, "Close prevents another read from starting")
}
