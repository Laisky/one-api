package anthropic

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientCompatibilityClaudeMCPAdmissionProof requires the existing private
// proof's complete no-receipt envelope and a clean body Close; status is not proof.
func TestClientCompatibilityClaudeMCPAdmissionProof(t *testing.T) {
	valid := `{"type":"error","error":{"type":"invalid_request_error","message":"synthetic pre-work rejection"}}`
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		proof := ParseClaudeMessageAdmissionRejection([]byte(valid), status, nil)
		require.True(t, IsAdmissionRejection(proof))
		require.Equal(t, http.StatusBadRequest, proof.StatusCode)
	}
	for _, tc := range []struct {
		name, body string
		status     int
		closeErr   error
	}{
		{"close_failed", valid, http.StatusOK, errors.New("synthetic close failure")},
		{"conflicting_status", valid, http.StatusInternalServerError, nil},
		{"malformed", valid + "{", http.StatusOK, nil},
		{"empty_usage", `{"type":"error","usage":{},"error":{"type":"invalid_request_error"}}`, http.StatusOK, nil},
		{"invalid_usage", `{"type":"error","usage":{"input_tokens":-1},"error":{"type":"invalid_request_error"}}`, http.StatusOK, nil},
		{"unknown_error", `{"type":"error","error":{"type":"future_error"}}`, http.StatusOK, nil},
		{"message_not_rejection", `{"type":"message","error":{"type":"invalid_request_error"}}`, http.StatusOK, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Nil(t, ParseClaudeMessageAdmissionRejection([]byte(tc.body), tc.status, tc.closeErr))
		})
	}
}
