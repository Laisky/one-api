package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test that an Anthropic SSE error event parses into StreamResponse with Error fields populated.
func TestParseStreamErrorEvent(t *testing.T) {
	payload := `{"type":"error","error":{"details":null,"type":"overloaded_error","message":"Overloaded"},"request_id":"req_011CSoQiKNJjFPYGZGMask1g"}`
	var sr StreamResponse
	err := json.Unmarshal([]byte(payload), &sr)
	require.NoError(t, err, "unmarshal failed")
	require.Equal(t, "error", sr.Type, "expected type error")
	require.Equal(t, "overloaded_error", string(sr.Error.Type), "expected error.type overloaded_error")
	require.Equal(t, "Overloaded", sr.Error.Message, "expected error.message Overloaded")
	require.NotEmpty(t, sr.RequestId, "expected request_id to be set")
}
