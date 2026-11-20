package model

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewUpstreamErrorAttribution requires the canonical constructor to produce
// exactly the fields the adaptors used to build by hand (status, message, type,
// code, Param "", RawError carrying only the message) and to mark the error as
// provider-caused, so a provider-supplied one_api_error type never reads as
// one-api's own error. The client-visible JSON must not change.
func TestNewUpstreamErrorAttribution(t *testing.T) {
	got := NewUpstreamError(http.StatusForbidden, "upstream key quota exhausted", ErrorTypeOneAPI, "insufficient_user_quota")
	require.Equal(t, http.StatusForbidden, got.StatusCode)
	require.Equal(t, "upstream key quota exhausted", got.Message)
	require.Equal(t, ErrorTypeOneAPI, got.Type)
	require.Equal(t, "insufficient_user_quota", got.Code)
	require.Empty(t, got.Param)
	require.False(t, got.MessageContainsRawBody)
	require.True(t, got.FromUpstream)
	require.False(t, got.IsOneAPIError())
	require.EqualError(t, got.RawError, "upstream key quota exhausted")

	encoded, err := json.Marshal(got.Error)
	require.NoError(t, err)
	require.JSONEq(t, `{"message":"upstream key quota exhausted","type":"one_api_error","param":"","code":"insufficient_user_quota"}`, string(encoded))
}

// TestNewUpstreamErrorAttributionOptions requires WithUpstreamErrorParam to set
// only Param, nil options to be ignored, and non-string or nil codes to be kept
// exactly as given.
func TestNewUpstreamErrorAttributionOptions(t *testing.T) {
	got := NewUpstreamError(http.StatusTooManyRequests, "Requests rate limit exceeded", ErrorType("Throttling"), 429,
		nil, WithUpstreamErrorParam("req-1"))
	require.Equal(t, http.StatusTooManyRequests, got.StatusCode)
	require.Equal(t, "Requests rate limit exceeded", got.Message)
	require.Equal(t, ErrorType("Throttling"), got.Type)
	require.Equal(t, 429, got.Code)
	require.Equal(t, "req-1", got.Param)
	require.True(t, got.FromUpstream)
	require.EqualError(t, got.RawError, "Requests rate limit exceeded")

	empty := NewUpstreamError(http.StatusBadGateway, "", ErrorTypeUnknown, nil)
	require.Nil(t, empty.Code)
	require.True(t, empty.FromUpstream)
	require.EqualError(t, empty.RawError, "")
}
