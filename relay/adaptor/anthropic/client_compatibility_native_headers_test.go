package anthropic

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// clientCompatibilityErrorBodyReader exposes a partial read failure followed by
// its untouched suffix. Parameters: none. Returns: a synthetic readable body.
type clientCompatibilityErrorBodyReader struct {
	first  bool
	closed bool
}

// Read returns the fixture prefix once with an error, then its suffix and EOF.
// Parameters: p receives bytes. Returns: copied bytes and the scripted error.
func (r *clientCompatibilityErrorBodyReader) Read(p []byte) (int, error) {
	if !r.first {
		r.first = true
		return copy(p, "prefix"), errors.New("synthetic read failure")
	}
	return copy(p, "suffix"), io.EOF
}

// Close marks the source closed. Parameters: none. Returns: no error.
func (r *clientCompatibilityErrorBodyReader) Close() error { r.closed = true; return nil }

// TestClientCompatibilityNativeErrorCaptureBoundary verifies a bounded read
// cannot lose bytes or ownership of the original body on fallback. Parameters:
// t owns tiny fixtures. Returns: nothing; no large test allocation is needed.
func TestClientCompatibilityNativeErrorCaptureBoundary(t *testing.T) {
	body := `{"type":"error","error":{"type":"invalid_request_error","message":"fixture"},"opaque":9007199254740993}`
	for _, tc := range []struct {
		name  string
		limit int64
		input string
		valid bool
	}{
		{"exact_boundary", int64(len(body)), body, true},
		{"oversize", int64(len(body) - 1), body, false},
		{"non_native", 128, `{"error":{"message":"generic"}}`, false},
		{"malformed", 128, `{"type":"error"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := &http.Response{StatusCode: 429, Header: make(http.Header), Body: io.NopCloser(bytes.NewBufferString(tc.input))}
			capture := prepareNativeErrorResponse(response, tc.limit)
			require.Equal(t, tc.valid, capture != nil)
			restored, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, tc.input, string(restored))
			require.NoError(t, response.Body.Close())
		})
	}
	t.Run("read_failure", func(t *testing.T) {
		original := &clientCompatibilityErrorBodyReader{}
		response := &http.Response{StatusCode: 429, Header: make(http.Header), Body: original}
		require.Nil(t, prepareNativeErrorResponse(response, 128))
		require.False(t, original.closed)
		restored, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, "prefixsuffix", string(restored))
		require.NoError(t, response.Body.Close())
		require.True(t, original.closed)
	})
}

// TestClientCompatibilityClaudeHeaderBoundary retains open-ended protocol
// metadata while respecting hop-by-hop fields and selected header isolation.
// Parameters: t owns synthetic values. Returns: nothing; copies do not alias.
func TestClientCompatibilityClaudeHeaderBoundary(t *testing.T) {
	source := http.Header{"Anthropic-Future": []string{"first", "second"}, "Anthropic-Workspace-Id": []string{"workspace-fixture"}, "Authorization": []string{"caller-secret"}, "X-Api-Key": []string{"caller-key"}, "Connection": []string{"anthropic-hop, request-id"}, "Anthropic-Hop": []string{"hop"}, "Request-Id": []string{"hop-id"}, "Retry-After": []string{"3"}, "Set-Cookie": []string{"private=fixture"}}
	destination := make(http.Header)
	copyClaudeProtocolHeaders(destination, source)
	require.Equal(t, []string{"first", "second"}, destination.Values("anthropic-future"))
	require.Equal(t, "workspace-fixture", destination.Get("anthropic-workspace-id"))
	require.Empty(t, destination.Get("anthropic-hop"))
	require.Empty(t, destination.Get("Authorization"))
	require.Empty(t, destination.Get("x-api-key"))
	destination["Anthropic-Future"][0] = "changed"
	require.Equal(t, "first", source.Get("anthropic-future"))
	selected := make(http.Header)
	CopyClaudeResponseHeaders(selected, source)
	require.Equal(t, "3", selected.Get("retry-after"))
	require.Empty(t, selected.Get("request-id"))
	require.Empty(t, selected.Get("Set-Cookie"))
	require.Empty(t, selected.Get("anthropic-future"))
}

// TestClientCompatibilityNativeErrorIdentity prevents old or mutated failures
// from authorizing exact native bytes at the final writer. Parameters: t owns
// contexts and candidates. Returns: nothing; only the original failure commits.
func TestClientCompatibilityNativeErrorIdentity(t *testing.T) {
	body := `{"type":"error","error":{"type":"rate_limit_error","message":"fixture"}}`
	for _, name := range []string{"matching", "different_pointer", "changed_status", "already_written"} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			candidate := prepareNativeErrorResponse(&http.Response{StatusCode: 429, Header: http.Header{"Request-Id": []string{"req_fixture"}}, Body: io.NopCloser(bytes.NewBufferString(body))}, 1024)
			require.NotNil(t, candidate)
			failure := &model.ErrorWithStatusCode{StatusCode: 429}
			RetainNativeErrorResponse(c, candidate, failure)
			switch name {
			case "different_pointer":
				failure = &model.ErrorWithStatusCode{StatusCode: 429}
			case "changed_status":
				failure.StatusCode = 400
			case "already_written":
				_, err := c.Writer.WriteString("committed")
				require.NoError(t, err)
			}
			written := WriteNativeErrorResponse(c, failure)
			require.Equal(t, name == "matching", written)
			value, _ := c.Get(ctxkey.ClaudeNativeErrorResponse)
			require.Nil(t, value)
			if written {
				require.Equal(t, 429, w.Code)
				require.Equal(t, body, w.Body.String())
				require.Equal(t, "req_fixture", w.Header().Get("request-id"))
			} else {
				require.Empty(t, w.Header().Get("request-id"))
			}
		})
	}
}
