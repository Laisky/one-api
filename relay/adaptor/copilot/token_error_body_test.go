package copilot

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
)

// copilotErrorBodySecret stands for data in a token-exchange response that must
// never reach an API caller: the token error text becomes the client-visible
// do_request_failed message.
const copilotErrorBodySecret = "internal-account-123"

// copilotRoundTripFunc serves an HTTP request with a function.
type copilotRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper for copilotRoundTripFunc.
//
// Parameters:
//   - req: the outgoing request.
//
// Returns:
//   - *http.Response: the synthetic response.
//   - error: the synthetic transport error, if any.
func (f copilotRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// copilotCountingBody serves remaining bytes of 'x' and records how many bytes
// were consumed, proving the token-endpoint read is bounded.
type copilotCountingBody struct {
	remaining int64
	read      atomic.Int64
}

// Read implements io.Reader for copilotCountingBody.
//
// Parameters:
//   - p: the destination buffer.
//
// Returns:
//   - int: the number of bytes written to p.
//   - error: io.EOF once the configured size was served.
func (b *copilotCountingBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), b.remaining)
	for i := range n {
		p[i] = 'x'
	}
	b.remaining -= n
	b.read.Add(n)
	return int(n), nil
}

// Close implements io.Closer for copilotCountingBody.
//
// Returns:
//   - error: always nil.
func (b *copilotCountingBody) Close() error { return nil }

// withCopilotTokenEndpoint points the token exchange at a synthetic response
// for the duration of the test.
//
// Parameters:
//   - t: the running test.
//   - status: the endpoint's HTTP status.
//   - contentType: the endpoint's Content-Type header.
//   - body: the endpoint's response body.
func withCopilotTokenEndpoint(t *testing.T, status int, contentType string, body io.ReadCloser) {
	t.Helper()
	old := client.ImpatientHTTPClient
	client.ImpatientHTTPClient = &http.Client{Transport: copilotRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {contentType}}, Body: body, Request: req}, nil
	})}
	t.Cleanup(func() { client.ImpatientHTTPClient = old })
}

// TestFetchCopilotTokenErrorBodyNeverEchoed requires token-exchange failures to
// carry a parsed reason or a generic description, never the raw endpoint body
// (OWASP API8:2023 / API10:2023), through the public GetCopilotAPIToken path.
func TestFetchCopilotTokenErrorBodyNeverEchoed(t *testing.T) {
	for i, tc := range []struct {
		name, contentType, body, want string
		status                        int
	}{
		{"github_bad_credentials", "application/json; charset=utf-8",
			`{"message":"Bad credentials","account":"` + copilotErrorBodySecret + `","documentation_url":"https://docs.github.com/rest"}`,
			"copilot token endpoint: Bad credentials", http.StatusUnauthorized},
		{"unknown_json", "application/json", `{"private":"` + copilotErrorBodySecret + `"}`,
			"copilot token endpoint: upstream returned HTTP 403 with an unrecognized error body (content-type application/json, 34 bytes)", http.StatusForbidden},
		{"html", "text/html", "<html>" + copilotErrorBodySecret + "</html>",
			"copilot token endpoint: upstream returned HTTP 502 with an unrecognized error body (content-type text/html, 33 bytes)", http.StatusBadGateway},
		{"empty", "text/plain", "", "copilot token endpoint: upstream returned HTTP 503 with an empty error body", http.StatusServiceUnavailable},
		{"missing_token", "application/json", `{"endpoints":{"api":"https://` + copilotErrorBodySecret + `.example.invalid"}}`,
			"copilot token endpoint response has no token", http.StatusOK},
		{"missing_token_with_message", "application/json", `{"message":"feature disabled","endpoints":{"api":"https://` + copilotErrorBodySecret + `.example.invalid"}}`,
			"copilot token endpoint response has no token: feature disabled", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withCopilotTokenEndpoint(t, tc.status, tc.contentType, io.NopCloser(strings.NewReader(tc.body)))
			_, err := GetCopilotAPIToken(context.Background(), 900000+i, "fixture-only")
			require.Error(t, err)
			require.NotContains(t, err.Error(), copilotErrorBodySecret)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestFetchCopilotTokenErrorBodyBoundedRead requires the token-endpoint read to
// stop one byte past the 1 MiB limit and an over-limit body to be described
// without parsing (OWASP API4:2023 unrestricted resource consumption).
func TestFetchCopilotTokenErrorBodyBoundedRead(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusOK} {
		body := &copilotCountingBody{remaining: 8 << 20}
		withCopilotTokenEndpoint(t, status, "text/plain", body)
		_, _, err := fetchCopilotToken(context.Background(), "fixture-only")
		require.Error(t, err)
		require.LessOrEqual(t, body.read.Load(), int64(1<<20)+1, "status %d", status)
		require.Less(t, len(err.Error()), 300, "the error text must not carry the body")
		require.Contains(t, err.Error(), "1048576-byte limit")
	}
}
