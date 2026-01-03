package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestErrorTypeJSONRoundTrip verifies that ErrorType values marshal and
// unmarshal as their underlying string representations.
func TestErrorTypeJSONRoundTrip(t *testing.T) {
	t.Parallel()
	t.Helper()

	original := Error{
		Message: "something went wrong",
		Type:    ErrorTypeOneAPI,
		Code:    "example_code",
	}

	payload, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Error
	require.NoError(t, json.Unmarshal(payload, &decoded))

	require.Equal(t, original.Type, decoded.Type)
	require.Equal(t, original.Message, decoded.Message)
	require.Equal(t, original.Code, decoded.Code)
}
