package common

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

var errCustomEventWrite = errors.New("custom event write failed")

// customEventResponseWriter records headers and delegates writes to the configured function.
type customEventResponseWriter struct {
	header  http.Header
	writeFn func([]byte) (int, error)
}

// Header returns the response headers owned by the test writer.
func (w *customEventResponseWriter) Header() http.Header {
	return w.header
}

// Write delegates one payload to the behavior configured by the test.
func (w *customEventResponseWriter) Write(payload []byte) (int, error) {
	return w.writeFn(payload)
}

// WriteHeader accepts the response status because CustomEvent only owns the response body.
func (*customEventResponseWriter) WriteHeader(int) {}

// TestCustomEventRenderPropagatesWriteErrors verifies that body and delimiter failures reach Gin.
func TestCustomEventRenderPropagatesWriteErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		failMatch []byte
	}{
		{name: "body", failMatch: []byte("data: hello")},
		{name: "delimiter", failMatch: []byte("\n\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			writer := &customEventResponseWriter{
				header: make(http.Header),
				writeFn: func(payload []byte) (int, error) {
					if bytes.Contains(payload, tc.failMatch) {
						return 0, errCustomEventWrite
					}
					return len(payload), nil
				},
			}

			err := (CustomEvent{Data: "data: hello"}).Render(writer)
			require.ErrorIs(t, err, errCustomEventWrite)
		})
	}
}

// TestCustomEventRenderRejectsShortWrites verifies that truncated SSE frames are reported.
func TestCustomEventRenderRejectsShortWrites(t *testing.T) {
	t.Parallel()
	writer := &customEventResponseWriter{
		header: make(http.Header),
		writeFn: func(payload []byte) (int, error) {
			return len(payload) - 1, nil
		},
	}

	err := (CustomEvent{Data: "data: hello"}).Render(writer)
	require.ErrorIs(t, err, io.ErrShortWrite)
}

// TestCustomEventRenderAcceptsNonStringData verifies that the exported any field cannot panic rendering.
func TestCustomEventRenderAcceptsNonStringData(t *testing.T) {
	t.Parallel()
	written := make([]byte, 0, 2)
	writer := &customEventResponseWriter{
		header: make(http.Header),
		writeFn: func(payload []byte) (int, error) {
			written = append(written, payload...)
			return len(payload), nil
		},
	}

	var renderErr error
	require.NotPanics(t, func() {
		renderErr = (CustomEvent{Data: 42}).Render(writer)
	})
	require.NoError(t, renderErr)
	require.Equal(t, "42", string(written))
}
