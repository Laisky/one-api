package typesafe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestProviderStatusErrorIsUpstream verifies that a TypeSafe HTTP failure keeps
// its client JSON byte-for-byte and its billing evidence unchanged, while being
// attributed to the provider rather than to one-api.
func TestProviderStatusErrorIsUpstream(t *testing.T) {
	estimated := EstimatedUsage("typesafe_missing_or_invalid_receipt")
	for _, tc := range []struct {
		name, body string
		status     int
		wantUsage  *model.Usage
	}{
		// Admission rejections are refunded; ambiguous 5xx keep the estimate.
		{"rate_limited", `{"detail":"rate limited"}`, http.StatusTooManyRequests, &model.Usage{}},
		{"overloaded", `{"detail":"overloaded"}`, StatusOverloaded, &model.Usage{}},
		{"server_error", `{"detail":"server error"}`, http.StatusInternalServerError, estimated},
		{"unavailable", `{"detail":"unavailable"}`, http.StatusServiceUnavailable, estimated},
		// A measured receipt outranks the status and must still be billed.
		{"unavailable_with_receipt", `{"detail":"late failure","usage":{"input_tokens":312,"output_tokens":0}}`,
			http.StatusServiceUnavailable, &model.Usage{PromptTokens: 312, TotalTokens: 312}},
		{"rate_limited_with_receipt", `{"detail":"late failure","usage":{"input_tokens":312,"output_tokens":0}}`,
			http.StatusTooManyRequests, &model.Usage{PromptTokens: 312, TotalTokens: 312}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, writer := responseContext(t)
			result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: tc.status,
				Header: http.Header{"Retry-After": {"10"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil)
			require.NotNil(t, apiErr)
			require.Equal(t, tc.status, apiErr.StatusCode)
			encoded, err := json.Marshal(apiErr)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf(`{"message":"TypeSafe returned HTTP %d","type":"one_api_error","param":"","code":"typesafe_upstream_error","status_code":%d}`, tc.status, tc.status), string(encoded))
			require.Equal(t, tc.wantUsage, usage)
			require.NotNil(t, result)
			result.Write(c)
			require.Equal(t, tc.status, writer.Code)
			require.Equal(t, tc.body, writer.Body.String(), "native error body must be forwarded verbatim")
			require.True(t, apiErr.FromUpstream, "a provider HTTP status must carry upstream provenance")
			require.False(t, apiErr.IsOneAPIError())
		})
	}
}

// TestLocalResponseFailuresStayLocal verifies that failures one-api detects
// itself while reading a TypeSafe response remain local one_api_error values.
func TestLocalResponseFailuresStayLocal(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		status     int
		body       io.ReadCloser
	}{
		{"body_read_failure", "typesafe_response_read_failed", http.StatusOK, &trackedBody{Reader: failingReader{}}},
		{"malformed_success_body", "typesafe_invalid_response", http.StatusOK, io.NopCloser(strings.NewReader(`<html>proxy error</html>`))},
		{"invalid_status_class", "typesafe_invalid_status", http.StatusFound, io.NopCloser(strings.NewReader(`{}`))},
		{"non_ok_success_status", "typesafe_invalid_status", http.StatusCreated, io.NopCloser(strings.NewReader(`{}`))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := responseContext(t)
			result, _, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: tc.status, Body: tc.body}, nil)
			require.Nil(t, result)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.Equal(t, tc.code, apiErr.Code)
			require.False(t, apiErr.FromUpstream)
			require.True(t, apiErr.IsOneAPIError())
		})
	}
}
