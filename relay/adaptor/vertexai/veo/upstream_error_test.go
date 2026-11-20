package veo

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// veoLeakMarker is a stand-in for provider-internal data that must never
// reach a client-visible message.
const veoLeakMarker = "SECRET-INTERNAL-TOKEN"

// veoErrorBodyCases lists provider error bodies with the client message each
// must produce for HTTP 429.
var veoErrorBodyCases = []struct {
	name string
	body string
	want string
}{
	{"google envelope", `{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"},"debug":"` + veoLeakMarker + `"}`, "Quota exceeded"},
	{"envelope array", `[{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"}}]`, "Quota exceeded"},
	{"status only", `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","details":"` + veoLeakMarker + `"}}`, "upstream returned HTTP 429 (RESOURCE_EXHAUSTED)"},
	{"unsafe status", `{"error":{"status":"<b>` + veoLeakMarker + `</b>"}}`, "upstream returned HTTP 429"},
	{"string error", `{"error":"` + veoLeakMarker + `"}`, "upstream returned HTTP 429"},
	{"html", `<html>` + veoLeakMarker + `</html>`, "upstream returned HTTP 429"},
	{"empty", ``, "upstream returned HTTP 429"},
	{"oversized", `{"error":{"message":"` + veoLeakMarker + strings.Repeat("x", 2<<20) + `"}}`, "upstream returned HTTP 429"},
}

// TestDoResponse_CreateErrorBodyIsNeverEchoed drives rejected creates through
// DoResponse and verifies the message is the parsed provider message or the
// generic status message, never raw body bytes.
func TestDoResponse_CreateErrorBodyIsNeverEchoed(t *testing.T) {
	t.Parallel()
	for _, tc := range veoErrorBodyCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := newFakeVertex(t, http.StatusTooManyRequests, tc.body)
			usage, got := (&Adaptor{}).DoResponse(newVeoTestContext(t), provider.createLikeRelay(t), veoChatMeta())
			require.Nil(t, usage)
			requireVeoUpstreamProvenance(t, got, tc.want, "veo_api_error", http.StatusTooManyRequests)
			require.NotContains(t, got.Message, veoLeakMarker)
		})
	}
}

// TestDoResponse_PollErrorBodyIsNeverEchoed drives accepted creates whose poll
// is rejected and verifies the poll message carries only the sanitized text.
func TestDoResponse_PollErrorBodyIsNeverEchoed(t *testing.T) {
	t.Parallel()
	for _, tc := range veoErrorBodyCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, pollReply{status: http.StatusTooManyRequests, body: tc.body})
			_, got := fastAdaptor().DoResponse(newVeoTestContext(t), provider.createLikeRelay(t), veoChatMeta())
			requireVeoUpstreamProvenance(t, got, "veo operation op-1 was accepted but polling failed: "+tc.want,
				"poll_request_failed_status", http.StatusTooManyRequests)
			require.NotContains(t, got.Message, veoLeakMarker)
		})
	}
}

// TestSanitizeProviderMessage verifies control characters are neutralized,
// invalid UTF-8 is rejected and long messages are bounded on a rune boundary.
func TestSanitizeProviderMessage(t *testing.T) {
	t.Parallel()
	require.Equal(t, "line one  line two", sanitizeProviderMessage(" line one\r\nline two\t"))
	require.Empty(t, sanitizeProviderMessage("bad \xff utf8"))
	require.Empty(t, sanitizeProviderMessage("   "))

	long := strings.Repeat("é", providerMessageLimit)
	bounded := sanitizeProviderMessage(long)
	require.LessOrEqual(t, len(bounded), providerMessageLimit)
	require.Equal(t, strings.Repeat("é", providerMessageLimit/2), bounded)
}

// TestReadBoundedBody verifies the limit, the overflow flag and read errors.
func TestReadBoundedBody(t *testing.T) {
	t.Parallel()
	data, tooLarge, err := readBoundedBody(strings.NewReader("12345"), 5)
	require.NoError(t, err)
	require.False(t, tooLarge)
	require.Equal(t, "12345", string(data))

	data, tooLarge, err = readBoundedBody(strings.NewReader("123456"), 5)
	require.NoError(t, err)
	require.True(t, tooLarge)
	require.Equal(t, "12345", string(data))

	source := &veoCountingReader{Reader: strings.NewReader(strings.Repeat("x", 4096))}
	_, tooLarge, err = readBoundedBody(source, 100)
	require.NoError(t, err)
	require.True(t, tooLarge)
	require.LessOrEqual(t, source.read, 101, "an oversized body must not be read past limit+1")

	_, _, err = readBoundedBody(veoFailingReadCloser{}, 5)
	require.ErrorContains(t, err, "read upstream response body")

	data, tooLarge, err = readBoundedBody(nil, 5)
	require.NoError(t, err)
	require.False(t, tooLarge)
	require.Empty(t, data)
}

// TestUpstreamErrorMessageFormat pins the generic message format.
func TestUpstreamErrorMessageFormat(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusBadRequest, http.StatusBadGateway} {
		require.Equal(t, fmt.Sprintf("upstream returned HTTP %d", status), upstreamErrorMessage([]byte("nope"), status))
	}
}

// veoCountingReader counts the bytes read through it. It is used from one
// goroutine only.
type veoCountingReader struct {
	*strings.Reader
	read int
}

// Read reads into p from the wrapped reader and records the byte count.
func (r *veoCountingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}
