package imagen

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// imagenLeakMarker is a stand-in for provider-internal data that must never
// reach a client-visible message.
const imagenLeakMarker = "SECRET-INTERNAL-TOKEN"

// TestDoResponse_ErrorBodyIsNeverEchoed drives generation and edit responses
// with assorted provider error bodies through DoResponse and verifies the
// message is the parsed Google error message or the generic status message,
// never raw body bytes, with provider attribution kept.
func TestDoResponse_ErrorBodyIsNeverEchoed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want string
	}{
		{"google envelope", `{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"},"debug":"` + imagenLeakMarker + `"}`, "Quota exceeded"},
		{"envelope array", `[{"error":{"code":429,"message":"Quota exceeded"}}]`, "Quota exceeded"},
		{"status only", `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":"` + imagenLeakMarker + `"}}`, "upstream returned HTTP 429 (RESOURCE_EXHAUSTED)"},
		{"html", `<html>` + imagenLeakMarker + `</html>`, "upstream returned HTTP 429"},
		{"control characters", `{"error":{"message":"line one\nline two"}}`, "line one line two"},
		{"oversized", `{"error":{"message":"` + imagenLeakMarker + strings.Repeat("x", 2<<20) + `"}}`, "upstream returned HTTP 429"},
	}
	for _, mode := range []int{relaymode.ImagesGenerations, relaymode.ImagesEdits} {
		for _, tc := range cases {
			t.Run(tc.name+" mode "+relaymode.String(mode), func(t *testing.T) {
				t.Parallel()
				resp := fetchTolerantImagenResponse(t, http.StatusTooManyRequests, tc.body)
				usage, got := (&Adaptor{}).DoResponse(newImagenTestContext(t), resp, &meta.Meta{Mode: mode})
				require.Nil(t, usage)
				requireUpstreamProvenance(t, got, tc.want, "imagen_api_error", http.StatusTooManyRequests)
				require.NotContains(t, got.Message, imagenLeakMarker)
			})
		}
	}
}

// fetchTolerantImagenResponse starts a fake Imagen provider answering with
// status and body and returns the live response. Unlike
// fetchImagenProviderResponse it ignores write failures, because the bounded
// reader may hang up before an oversized body is fully written. It accepts t,
// status, and body.
func fetchTolerantImagenResponse(t *testing.T, status int, body string) *http.Response {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Post(srv.URL+"/v1/projects/p/locations/us-central1/publishers/google/models/imagen-4.0-generate-001:predict",
		"application/json", strings.NewReader(`{"instances":[{"prompt":"cat"}]}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// countingCloser wraps a reader and counts Close calls.
type countingCloser struct {
	io.Reader
	closed atomic.Int32
}

// Close records the call and reports success.
func (c *countingCloser) Close() error {
	c.closed.Add(1)
	return nil
}

// TestReadImagenResponse_SuccessBodyIsBounded verifies a successful body over
// maxSuccessBodyBytes fails locally instead of being buffered without bound,
// and that the body is closed.
func TestReadImagenResponse_SuccessBodyIsBounded(t *testing.T) {
	t.Parallel()
	body := &countingCloser{Reader: io.LimitReader(zeroReader{}, maxSuccessBodyBytes+2)}
	resp := &http.Response{StatusCode: http.StatusOK, Body: body}

	data, got := readImagenResponse(newImagenTestContext(t), resp)
	require.Nil(t, data)
	requireLocalProvenance(t, got, "response_body_too_large", http.StatusBadGateway)
	require.Equal(t, int32(1), body.closed.Load())
}

// TestReadImagenResponse_ErrorBodyIsBoundedToOneMiB verifies an error body
// is read only up to maxErrorBodyBytes and yields no body for parsing.
func TestReadImagenResponse_ErrorBodyIsBoundedToOneMiB(t *testing.T) {
	t.Parallel()
	source := &countingReader{Reader: io.LimitReader(zeroReader{}, 8<<20)}
	resp := &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(source)}

	data, got := readImagenResponse(newImagenTestContext(t), resp)
	require.Nil(t, got)
	require.Nil(t, data)
	require.LessOrEqual(t, source.read.Load(), int64(maxErrorBodyBytes+1))
}

// zeroReader yields an endless stream of zero bytes.
type zeroReader struct{}

// Read fills p with zero bytes and returns len(p).
func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// countingReader counts the bytes read through it.
type countingReader struct {
	io.Reader
	read atomic.Int64
}

// Read reads into p from the wrapped reader and records the byte count.
func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read.Add(int64(n))
	return n, err
}

// TestSanitizeProviderMessage verifies invalid UTF-8 is rejected and long
// messages are bounded on a rune boundary.
func TestSanitizeProviderMessage(t *testing.T) {
	t.Parallel()
	require.Empty(t, sanitizeProviderMessage("bad \xff utf8"))
	bounded := sanitizeProviderMessage(strings.Repeat("é", providerMessageLimit))
	require.Equal(t, strings.Repeat("é", providerMessageLimit/2), bounded)
}
