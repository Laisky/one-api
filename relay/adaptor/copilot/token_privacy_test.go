package copilot

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
)

// tokenPrivacyTransport supplies synthetic token responses without network I/O.
type tokenPrivacyTransport func(*http.Request) (*http.Response, error)

// RoundTrip returns the in-memory response selected by the fixture.
func (f tokenPrivacyTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// TestCopilotTokenErrorsDoNotEchoUpstreamBody verifies known credential-response
// payloads are selected for omission at source, rather than guessed from text.
func TestCopilotTokenErrorsDoNotEchoUpstreamBody(t *testing.T) {
	old := client.ImpatientHTTPClient
	t.Cleanup(func() { client.ImpatientHTTPClient = old })
	const canary = "SYNTHETIC_PRIVATE_UPSTREAM_BODY"
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"rejected", 403, canary},
		{"missing token", 200, `{"error":"SYNTHETIC_PRIVATE_UPSTREAM_BODY"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client.ImpatientHTTPClient = &http.Client{Transport: tokenPrivacyTransport(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "/copilot_internal/v2/token", req.URL.Path)
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: req}, nil
			})}
			_, _, err := fetchCopilotToken(context.Background(), "SYNTHETIC_GITHUB_TOKEN")
			require.Error(t, err)
			require.NotContains(t, err.Error(), canary)
			require.Contains(t, err.Error(), "response")
			require.Contains(t, err.Error(), "body_bytes="+strconv.Itoa(len(tc.body)))
		})
	}
}

// TestCopilotTokenSuccessRetainsProviderToken verifies source redaction touches
// diagnostics only; successful credential parsing and return values are unchanged.
func TestCopilotTokenSuccessRetainsProviderToken(t *testing.T) {
	old := client.ImpatientHTTPClient
	t.Cleanup(func() { client.ImpatientHTTPClient = old })
	client.ImpatientHTTPClient = &http.Client{Transport: tokenPrivacyTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
			`{"token":"SYNTHETIC_COPILOT_TOKEN","expires_in":600}`)), Request: req}, nil
	})}
	token, expiry, err := fetchCopilotToken(context.Background(), "SYNTHETIC_GITHUB_TOKEN")
	require.NoError(t, err)
	require.Equal(t, "SYNTHETIC_COPILOT_TOKEN", token)
	require.False(t, expiry.IsZero())
}
