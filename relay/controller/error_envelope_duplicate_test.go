package controller

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestRelayErrorEnvelopeDuplicateMembers requires that encoding/json's
// case-insensitive duplicate members (each occurrence calls UnmarshalJSON on the
// same receiver, null included) never discard a readable value: a null or
// contract-breaking duplicate is absent, while a later readable duplicate wins
// as for any encoding/json field. Losing the readable message used to send the
// whole raw body to the client.
func TestRelayErrorEnvelopeDuplicateMembers(t *testing.T) {
	upstream, defaultCode := model.ErrorTypeUpstream, "bad_response_status_code"
	for _, tc := range []struct {
		name, body, message string
		errorType           model.ErrorType
		code                any
		param               string
	}{
		{"flat_message_then_null", `{"message":"quota exceeded","Message":null}`, "quota exceeded", upstream, defaultCode, "429"},
		{"flat_message_then_object", `{"message":"quota exceeded","MESSAGE":{"x":1}}`, "quota exceeded", upstream, defaultCode, "429"},
		{"flat_message_then_number", `{"message":"quota exceeded","Message":7}`, "quota exceeded", upstream, defaultCode, "429"},
		{"flat_message_later_string_wins", `{"message":"first","Message":"second"}`, "second", upstream, defaultCode, "429"},
		{"flat_type_then_null", `{"message":"m","type":"TooManyRequests","Type":null}`, "m", "TooManyRequests", defaultCode, "429"},
		{"flat_code_then_null", `{"message":"m","code":"AccountQuotaExceeded","Code":null}`, "m", upstream, "AccountQuotaExceeded", "429"},
		{"flat_code_then_out_of_range", `{"message":"m","code":"AccountQuotaExceeded","Code":1e400}`, "m", upstream, "AccountQuotaExceeded", "429"},
		{"flat_code_then_object", `{"message":"m","code":"AccountQuotaExceeded","Code":{"x":1}}`, "m", upstream, "AccountQuotaExceeded", "429"},
		{"flat_param_then_null", `{"message":"m","param":"model","Param":null}`, "m", upstream, defaultCode, "model"},
		{"nested_error_then_null", `{"error":{"message":"quota exceeded","code":"AccountQuotaExceeded"},"Error":null}`, "quota exceeded", "", "AccountQuotaExceeded", ""},
		{"nested_error_then_number", `{"error":{"message":"quota exceeded","code":"AccountQuotaExceeded"},"ERROR":5}`, "quota exceeded", "", "AccountQuotaExceeded", ""},
		{"nested_code_then_null", `{"error":{"message":"quota exceeded","code":"AccountQuotaExceeded","Code":null}}`, "quota exceeded", "", "AccountQuotaExceeded", ""},
		{"nested_message_then_null", `{"error":{"message":"quota exceeded","Message":null,"type":"rate_limit_error"}}`, "quota exceeded", "rate_limit_error", nil, ""},
		{"nested_type_then_array", `{"error":{"message":"m","type":"rate_limit_error","Type":["x"]}}`, "m", "rate_limit_error", nil, ""},
		{"nested_objects_merge", `{"error":{"message":"m"},"Error":{"code":"c"}}`, "m", "", "c", ""},
		{"nested_string_then_null", `{"error":"model not found","Error":null}`, "model not found", upstream, defaultCode, "429"},
		{"header_then_null", `{"header":{"message":"capacity"},"Header":null}`, "capacity", upstream, defaultCode, "429"},
		{"header_message_then_null", `{"header":{"message":"capacity","Message":null}}`, "capacity", upstream, defaultCode, "429"},
		{"response_then_null", `{"response":{"error":{"message":"capacity"}},"Response":null}`, "capacity", upstream, defaultCode, "429"},
		{"response_error_then_null", `{"response":{"error":{"message":"capacity"},"Error":null}}`, "capacity", upstream, defaultCode, "429"},
		{"detail_then_list", `{"detail":"version missing","Detail":[{"msg":"x"}]}`, "version missing", upstream, defaultCode, "429"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RelayErrorHandler(&http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))})
			require.Equal(t, http.StatusTooManyRequests, got.StatusCode)
			require.Equal(t, tc.message, got.Message)
			require.Equal(t, tc.errorType, got.Type)
			require.Equal(t, tc.code, got.Code)
			require.Equal(t, tc.param, got.Param)
			require.True(t, got.FromUpstream)
			require.False(t, got.MessageContainsRawBody)
		})
	}
}

// TestRelayErrorEnvelopeNestedAttributionWithoutMessage requires a nested
// error object whose message is unusable to keep its own type, code and param
// (nested first, then flat keys, then defaults) instead of discarding them,
// while the message falls back to the flat keys or the generic description.
func TestRelayErrorEnvelopeNestedAttributionWithoutMessage(t *testing.T) {
	const secret = "internal-account-123"
	for _, tc := range []struct {
		name, body, message string
		errorType           model.ErrorType
		code                any
		param               string
	}{
		{"message_object_keeps_code", `{"error":{"message":{"x":"` + secret + `"},"type":"rate_limit","code":"quota_exceeded"}}`,
			"upstream returned HTTP 429 with an unrecognized error body (%d bytes)", "rate_limit", "quota_exceeded", "429"},
		{"missing_message_keeps_param", `{"error":{"type":"invalid_request_error","param":"temperature","code":400}}`,
			"upstream returned HTTP 429 with an unrecognized error body (%d bytes)", "invalid_request_error", float64(400), "temperature"},
		{"nested_code_wins_over_flat", `{"error":{"message":{"detail":"` + secret + `"},"code":"inner"},"message":"outer","code":"outer"}`,
			"outer", model.ErrorTypeUpstream, "inner", "429"},
		{"flat_fills_absent_nested", `{"error":{"code":"inner"},"message":"outer","type":"outer_type","param":"p"}`,
			"outer", "outer_type", "inner", "p"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RelayErrorHandler(&http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))})
			require.Equal(t, errorBodyExpected(tc.message, tc.body), got.Message)
			require.Equal(t, tc.errorType, got.Type)
			require.Equal(t, tc.code, got.Code)
			require.Equal(t, tc.param, got.Param)
			require.NotContains(t, got.Message, secret)
			require.False(t, got.MessageContainsRawBody)
		})
	}
}
