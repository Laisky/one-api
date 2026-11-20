package typesafe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// readFailureExpectedJSON is the client-visible JSON of an unreadable TypeSafe
// body; only its status may follow the provider.
const readFailureExpectedJSON = `{"message":"incomplete or oversized TypeSafe response","type":"one_api_error","param":"","code":"typesafe_response_read_failed","status_code":%d}`

// readFailureReceiptBody is a complete provider body carrying a measured
// receipt. When the read fails or exceeds the bound, the receipt must not be
// billed, exactly as before, even though the buffered bytes would parse.
const readFailureReceiptBody = `{"detail":"late failure","usage":{"input_tokens":312,"output_tokens":0}}`

// truncatedReceiptBody returns a body that yields a whole receipt-bearing error
// envelope and then fails, as a connection reset before EOF would.
//
// Parameters: none.
//
// Returns:
//   - *trackedBody: the interrupted body, recording whether it was closed.
func truncatedReceiptBody() *trackedBody {
	return &trackedBody{Reader: io.MultiReader(strings.NewReader(readFailureReceiptBody), failingReader{})}
}

// oversizedReceiptBody returns a receipt-bearing error envelope padded with
// trailing whitespace past the response buffering bound; the buffered prefix
// is still a valid JSON object with a valid receipt.
//
// Parameters: none.
//
// Returns:
//   - *trackedBody: the oversized body, recording whether it was closed.
func oversizedReceiptBody() *trackedBody {
	return &trackedBody{Reader: strings.NewReader(readFailureReceiptBody + strings.Repeat(" ", maxResponseBytes))}
}

// TestUnreadableProviderStatusBodyKeepsProviderStatus verifies that an
// interrupted or oversized body on a provider HTTP error status keeps the
// provider's status with provider provenance, keeps the read-failure code and
// message, records the upstream request id, and bills exactly as before: the
// status-derived refund or estimate, never a receipt from the unreadable body.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestUnreadableProviderStatusBodyKeepsProviderStatus(t *testing.T) {
	estimated := EstimatedUsage("typesafe_missing_or_invalid_receipt")
	for _, tc := range []struct {
		name      string
		status    int
		body      func() *trackedBody
		wantUsage *model.Usage
	}{
		{"truncated_401", http.StatusUnauthorized, truncatedReceiptBody, &model.Usage{}},
		{"truncated_429", http.StatusTooManyRequests, truncatedReceiptBody, &model.Usage{}},
		{"oversized_529", StatusOverloaded, oversizedReceiptBody, &model.Usage{}},
		{"truncated_500", http.StatusInternalServerError, truncatedReceiptBody, estimated},
		{"truncated_503", http.StatusServiceUnavailable, truncatedReceiptBody, estimated},
		{"oversized_503", http.StatusServiceUnavailable, oversizedReceiptBody, estimated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, writer := responseContext(t)
			body := tc.body()
			result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: tc.status,
				Header: http.Header{RequestIDHeader: {"req_read_failure"}}, Body: body}, nil)
			require.Nil(t, result, "an unreadable body is never forwarded")
			require.True(t, body.closed)
			require.Equal(t, tc.wantUsage, usage)
			require.Equal(t, "req_read_failure", c.GetString(ctxkey.UpstreamRequestId))
			require.NotNil(t, apiErr)
			require.Equal(t, tc.status, apiErr.StatusCode)
			encoded, err := json.Marshal(apiErr)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf(readFailureExpectedJSON, tc.status), string(encoded))
			require.NotContains(t, string(encoded), "late failure")
			require.True(t, apiErr.FromUpstream, "the provider's error status line carries upstream provenance")
			require.False(t, apiErr.IsOneAPIError())
			require.False(t, writer.Body.Len() > 0, "nothing is written before settlement")
		})
	}
}

// TestUnreadableSuccessBodyStaysLocal verifies the control: an interrupted or
// oversized body on HTTP 200 remains one-api's own 502 read failure and keeps
// the ambiguous-attempt estimate.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestUnreadableSuccessBodyStaysLocal(t *testing.T) {
	for name, newBody := range map[string]func() *trackedBody{"truncated": truncatedReceiptBody, "oversized": oversizedReceiptBody} {
		t.Run(name, func(t *testing.T) {
			c, _ := responseContext(t)
			body := newBody()
			result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: http.StatusOK, Body: body}, nil)
			require.Nil(t, result)
			require.True(t, body.closed)
			require.Equal(t, EstimatedUsage("typesafe_missing_or_invalid_receipt"), usage)
			require.NotNil(t, apiErr)
			encoded, err := json.Marshal(apiErr)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf(readFailureExpectedJSON, http.StatusBadGateway), string(encoded))
			require.False(t, apiErr.FromUpstream)
			require.True(t, apiErr.IsOneAPIError())
		})
	}
}
