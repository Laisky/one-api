package message

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// resendRedactionTransport returns only fixture responses and never opens a socket.
type resendRedactionTransport struct {
	status   int
	body     string
	requests int
}

// RoundTrip records one synthetic request and returns the configured provider body.
func (r *resendRedactionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.requests++
	return &http.Response{StatusCode: r.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(r.body)), Request: req}, nil
}

// TestResendRawBodyErrorsExplicitlyRedacted preserves parsed error reasons while
// preventing unparsed provider bodies from becoming alert-bearing error text.
func TestResendRawBodyErrorsExplicitlyRedacted(t *testing.T) {
	previousClient, previousURL, previousKey := resendHTTPClient, resendAPIURL, config.ResendAPIKey
	t.Cleanup(func() { resendHTTPClient, resendAPIURL, config.ResendAPIKey = previousClient, previousURL, previousKey })
	const secret = "fake-provider-secret"
	const numericSecret = "918273645546372819999"
	cases := []struct {
		name           string
		status         int
		body, category string
		ordinary       bool
	}{
		{"non_json_error", 502, "<html>" + secret + "</html>", "status 502", false},
		{"numeric_success_error", 200, `{"statusCode":` + numericSecret + `}`, "failed to decode Resend success response", false},
		{"invalid_success", 200, "{invalid-json " + secret, "failed to decode Resend success response", false},
		{"missing_id", 200, "{\"opaque\":\"" + secret + "\"}", "no message id", false},
		{"parsed_reason", 422, "{\"name\":\"validation_error\",\"message\":\"recipient is invalid\"}", "recipient is invalid", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &resendRedactionTransport{status: tc.status, body: tc.body}
			resendHTTPClient = &http.Client{Transport: transport}
			resendAPIURL = "https://fixture.invalid/emails"
			config.ResendAPIKey = "fake-key"
			err := postResendEmail("sender@example.test", []string{"recipient@example.test"}, "fixture", "fixture")
			require.Error(t, err)
			require.Equal(t, 1, transport.requests)
			require.Contains(t, err.Error(), tc.category)
			require.NotContains(t, err.Error(), secret)
			require.NotContains(t, fmt.Sprintf("%+v", err), numericSecret)
			if tc.ordinary {
				require.NotContains(t, err.Error(), logger.Redact(nil))
			} else {
				require.Contains(t, err.Error(), logger.Redact(tc.body))
				require.Contains(t, err.Error(), fmt.Sprintf("body_bytes=%d", len(tc.body)))
			}
		})
	}
}
