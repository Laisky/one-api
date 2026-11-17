package controller

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestRelayErrorEnvelopeMetadata requires equivalent flat and nested upstream
// attribution while retaining nested precedence and existing parser fallbacks.
func TestRelayErrorEnvelopeMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, body, message, errorType, param string
		code                                  any
		raw                                   bool
	}{
		{"flat", `{"message":"monthly quota","type":"TooManyRequests","code":"AccountQuotaExceeded","param":""}`, "monthly quota", "TooManyRequests", "", "AccountQuotaExceeded", false},
		{"nested", `{"error":{"message":"monthly quota","type":"TooManyRequests","code":"AccountQuotaExceeded","param":""}}`, "monthly quota", "TooManyRequests", "", "AccountQuotaExceeded", false},
		{"nested_precedence", `{"message":"outer","type":"outer","code":"outer","param":"outer","error":{"message":"inner","type":"inner","code":"inner","param":"inner"}}`, "inner", "inner", "inner", "inner", false},
		{"numeric_code", `{"message":"capacity","type":"limit","code":123}`, "capacity", "limit", "429", float64(123), false},
		{"message_only", `{"message":"capacity"}`, "capacity", string(model.ErrorTypeUpstream), "429", "bad_response_status_code", false},
		{"legacy_message", `{"msg":"capacity"}`, "capacity", string(model.ErrorTypeUpstream), "429", "bad_response_status_code", false},
		{"empty", `{}`, "bad response [429] {}", string(model.ErrorTypeUpstream), "429", "bad_response_status_code", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RelayErrorHandler(&http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(tc.body))})
			require.Equal(t, http.StatusTooManyRequests, got.StatusCode)
			require.Equal(t, tc.message, got.Message)
			require.Equal(t, model.ErrorType(tc.errorType), got.Type)
			require.Equal(t, tc.code, got.Code)
			require.Equal(t, tc.param, got.Param)
			require.Equal(t, tc.raw, got.MessageContainsRawBody)
			require.Error(t, got.RawError)
		})
	}
	for _, body := range []string{`{"message":`, `<html>busy</html>`} {
		got := RelayErrorHandler(&http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(body))})
		require.Equal(t, http.StatusTooManyRequests, got.StatusCode)
		require.Equal(t, "bad_response_status_code", got.Code)
		require.True(t, got.MessageContainsRawBody)
		require.Contains(t, got.Message, "failed to unmarshal response body")
	}
}
