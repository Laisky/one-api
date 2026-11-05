package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// claudeBodyFailure provides a typed transport failure for cause preservation checks.
type claudeBodyFailure struct{ message string }

// Error returns the transport failure message.
func (e *claudeBodyFailure) Error() string { return e.message }

// claudeFailingBody supplies distinct read and close errors to the converter.
type claudeFailingBody struct {
	readErr, closeErr error
	closeCount        int
}

// Read returns the selected failure or EOF without producing a response body.
func (b *claudeFailingBody) Read(_ []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return 0, io.EOF
}

// Close returns the selected close error and records body ownership.
func (b *claudeFailingBody) Close() error { b.closeCount++; return b.closeErr }

// TestClaudeResponseBodyFailureStack checks diagnostic frames and the API contract.
func TestClaudeResponseBodyFailureStack(t *testing.T) {
	for _, tc := range []struct {
		name, code  string
		readFailure bool
	}{
		{"read", "read_response_body_failed", true},
		{"close", "close_response_body_failed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := &claudeBodyFailure{message: "synthetic body failure"}
			body := &claudeFailingBody{}
			if tc.readFailure {
				body.readErr = cause
			} else {
				body.closeErr = cause
			}
			result, failure := (&Adaptor{}).convertToClaudeResponse(nil, &http.Response{Body: body}, nil)
			require.Nil(t, result)
			require.NotNil(t, failure)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			require.Equal(t, tc.code, failure.Code)
			require.Equal(t, model.ErrorTypeOneAPI, failure.Type)
			require.Equal(t, cause.Error(), failure.Message)
			require.ErrorIs(t, failure.RawError, cause)
			var typed *claudeBodyFailure
			require.True(t, errors.As(failure.RawError, &typed))
			require.Same(t, cause, typed)
			wire, err := json.Marshal(failure)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{"message":"synthetic body failure","type":"one_api_error","param":"","code":%q,"status_code":500}`, tc.code), string(wire))
			require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "convertToClaudeResponse")
			if tc.readFailure {
				require.Equal(t, 0, body.closeCount)
			} else {
				require.Equal(t, 1, body.closeCount)
			}
		})
	}
}
