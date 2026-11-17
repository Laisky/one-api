package controller

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// relayErrorEnvelopeCase describes one upstream error body and the normalized
// attribution RelayErrorHandler must report for it.
type relayErrorEnvelopeCase struct {
	name, body, message, errorType, param string
	code                                  any
	raw                                   bool
}

// runRelayErrorEnvelopeCases parses each body as an upstream HTTP 429 response
// and requires the normalized message, type, code, param and raw-body flag.
func runRelayErrorEnvelopeCases(t *testing.T, cases []relayErrorEnvelopeCase) {
	t.Helper()
	for _, tc := range cases {
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
}

// TestRelayErrorEnvelopeMetadata requires equivalent flat and nested upstream
// attribution while retaining nested precedence and existing parser fallbacks.
func TestRelayErrorEnvelopeMetadata(t *testing.T) {
	upstream, defaultCode := string(model.ErrorTypeUpstream), "bad_response_status_code"
	runRelayErrorEnvelopeCases(t, []relayErrorEnvelopeCase{
		{"flat", `{"message":"monthly quota","type":"TooManyRequests","code":"AccountQuotaExceeded","param":""}`, "monthly quota", "TooManyRequests", "", "AccountQuotaExceeded", false},
		{"nested", `{"error":{"message":"monthly quota","type":"TooManyRequests","code":"AccountQuotaExceeded","param":""}}`, "monthly quota", "TooManyRequests", "", "AccountQuotaExceeded", false},
		{"nested_precedence", `{"message":"outer","type":"outer","code":"outer","param":"outer","error":{"message":"inner","type":"inner","code":"inner","param":"inner"}}`, "inner", "inner", "inner", "inner", false},
		{"numeric_code", `{"message":"capacity","type":"limit","code":123}`, "capacity", "limit", "429", float64(123), false},
		{"message_only", `{"message":"capacity"}`, "capacity", upstream, "429", defaultCode, false},
		{"legacy_message", `{"msg":"capacity"}`, "capacity", upstream, "429", defaultCode, false},
		{"case_folded_keys", `{"Message":"capacity","Type":"limit","Code":"busy"}`, "capacity", "limit", "429", "busy", false},
		{"empty", `{}`, "bad response [429] {}", upstream, "429", defaultCode, true},
	})
	for _, body := range []string{`{"message":`, `<html>busy</html>`, `[1,2]`, `"busy"`} {
		got := RelayErrorHandler(&http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(body))})
		require.Equal(t, http.StatusTooManyRequests, got.StatusCode)
		require.Equal(t, "bad_response_status_code", got.Code)
		require.True(t, got.MessageContainsRawBody)
		require.Contains(t, got.Message, "failed to unmarshal response body")
	}
}

// TestRelayErrorEnvelopeTolerantMetadata is the regression suite for optional
// provider metadata whose JSON kind differs from the OpenAI string contract. A
// readable message must survive; the unusable field is treated as absent instead
// of turning the whole response into the raw-body fallback, which would echo
// provider internals such as the account identifier below to the client.
func TestRelayErrorEnvelopeTolerantMetadata(t *testing.T) {
	const secret = "internal-account-123"
	upstream, defaultCode := string(model.ErrorTypeUpstream), "bad_response_status_code"
	runRelayErrorEnvelopeCases(t, []relayErrorEnvelopeCase{
		{"flat_param_object", `{"message":"quota exceeded","type":"TooManyRequests","code":"AccountQuotaExceeded","param":{"account":"` + secret + `"}}`, "quota exceeded", "TooManyRequests", "429", "AccountQuotaExceeded", false},
		{"flat_param_array", `{"message":"quota exceeded","param":["` + secret + `"]}`, "quota exceeded", upstream, "429", defaultCode, false},
		{"flat_param_number", `{"message":"quota exceeded","param":7}`, "quota exceeded", upstream, "429", defaultCode, false},
		{"flat_param_null", `{"message":"quota exceeded","param":null}`, "quota exceeded", upstream, "429", defaultCode, false},
		{"flat_type_object", `{"message":"quota exceeded","type":{"name":"` + secret + `"},"code":"AccountQuotaExceeded"}`, "quota exceeded", upstream, "429", "AccountQuotaExceeded", false},
		{"flat_type_number", `{"message":"quota exceeded","type":429}`, "quota exceeded", upstream, "429", defaultCode, false},
		{"flat_code_null", `{"message":"quota exceeded","code":null}`, "quota exceeded", upstream, "429", defaultCode, false},
		{"flat_message_object_falls_back", `{"message":{"detail":"` + secret + `"},"msg":"capacity"}`, "capacity", upstream, "429", defaultCode, false},
		{"flat_header_string", `{"header":"` + secret + `","msg":"capacity"}`, "capacity", upstream, "429", defaultCode, false},
		{"flat_response_string", `{"response":"` + secret + `","err":"capacity"}`, "capacity", upstream, "429", defaultCode, false},
		{"nested_param_object", `{"error":{"message":"quota exceeded","type":"TooManyRequests","code":"AccountQuotaExceeded","param":{"account":"` + secret + `"}}}`, "quota exceeded", "TooManyRequests", "", "AccountQuotaExceeded", false},
		{"nested_type_array", `{"error":{"message":"quota exceeded","type":["` + secret + `"],"code":"AccountQuotaExceeded"}}`, "quota exceeded", "", "", "AccountQuotaExceeded", false},
		{"nested_message_object_uses_flat", `{"error":{"message":{"detail":"` + secret + `"},"code":"inner"},"message":"outer","code":"outer"}`, "outer", upstream, "429", "outer", false},
		{"error_string", `{"error":"model not found"}`, "model not found", upstream, "429", defaultCode, false},
		{"error_string_with_flat_code", `{"error":"model not found","code":"model_not_found","type":"invalid_request_error"}`, "model not found", "invalid_request_error", "429", "model_not_found", false},
		{"error_number_uses_flat", `{"error":500,"message":"capacity"}`, "capacity", upstream, "429", defaultCode, false},
		{"code_out_of_range", `{"message":"capacity","code":1e400}`, "capacity", upstream, "429", defaultCode, false},
		{"code_negative_float", `{"message":"capacity","code":-1.5}`, "capacity", upstream, "429", -1.5, false},
		{"code_bool_object", `{"error":{"message":"capacity","code":{"id":"` + secret + `"}},"code":true}`, "capacity", "", "", nil, false},
	})
	for _, body := range []string{
		`{"message":"quota exceeded","param":{"account":"` + secret + `"}}`,
		`{"error":{"message":"quota exceeded","param":{"account":"` + secret + `"}}}`,
		`{"message":{"detail":"` + secret + `"},"msg":"capacity"}`,
	} {
		got := RelayErrorHandler(&http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(body))})
		require.NotContains(t, got.Message, secret, "unusable optional metadata must never echo the raw provider body")
		require.NotContains(t, got.Param, secret)
		require.False(t, got.MessageContainsRawBody)
	}
}
