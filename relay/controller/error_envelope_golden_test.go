package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// relayErrorEnvelopeGolden pins the complete normalized result for one
// well-formed provider error body.
type relayErrorEnvelopeGolden struct {
	name   string
	status int
	body   string
	// clientJSON is the exact client-visible JSON of the normalized error.
	clientJSON string
	// rawError is the exact diagnostic RawError text.
	rawError string
}

// TestRelayErrorEnvelopeGoldenWellFormed pins, byte for byte, what
// RelayErrorHandler produced for well-formed OpenAI nested, flat
// OpenAI-compatible (#625), Anthropic and Google error envelopes before the
// disclosure and tolerance hardening: the hardening must change only
// unrecognized bodies. The goldens were captured from the unmodified handler.
func TestRelayErrorEnvelopeGoldenWellFormed(t *testing.T) {
	for _, tc := range []relayErrorEnvelopeGolden{
		{"openai_nested_rate_limit", 429,
			`{"error":{"message":"Rate limit reached for gpt-4o in organization org-x on tokens per min (TPM): Limit 30000, Used 29000.","type":"tokens","param":null,"code":"rate_limit_exceeded"}}`,
			`{"message":"Rate limit reached for gpt-4o in organization org-x on tokens per min (TPM): Limit 30000, Used 29000.","type":"tokens","param":"","code":"rate_limit_exceeded","status_code":429}`,
			"Rate limit reached for gpt-4o in organization org-x on tokens per min (TPM): Limit 30000, Used 29000."},
		{"openai_nested_param", 400,
			`{"error":{"message":"Invalid value for 'temperature': expected a number.","type":"invalid_request_error","param":"temperature","code":"invalid_value"}}`,
			`{"message":"Invalid value for 'temperature': expected a number.","type":"invalid_request_error","param":"temperature","code":"invalid_value","status_code":400}`,
			"Invalid value for 'temperature': expected a number."},
		{"openai_nested_auth", 401,
			`{"error":{"message":"Incorrect API key provided: sk-abc***. You can find your API key at https://platform.openai.com/account/api-keys.","type":"invalid_request_error","param":null,"code":"invalid_api_key"}}`,
			`{"message":"Incorrect API key provided: sk-abc***. You can find your API key at https://platform.openai.com/account/api-keys.","type":"invalid_request_error","param":"","code":"invalid_api_key","status_code":401}`,
			"Incorrect API key provided: sk-abc***. You can find your API key at https://platform.openai.com/account/api-keys."},
		{"openai_nested_numeric_code", 503,
			`{"error":{"message":"The server is overloaded","type":"server_error","param":"","code":503}}`,
			`{"message":"The server is overloaded","type":"server_error","param":"","code":503,"status_code":503}`,
			"The server is overloaded"},
		{"flat_625_empty_param", 402,
			`{"message":"You have exceeded the monthly usage quota","type":"TooManyRequests","code":"AccountQuotaExceeded","param":""}`,
			`{"message":"You have exceeded the monthly usage quota","type":"TooManyRequests","param":"","code":"AccountQuotaExceeded","status_code":402}`,
			"bad response 402"},
		{"flat_625_null_param", 429,
			`{"code":"AccountQuotaExceeded","message":"You have exceeded the monthly usage quota.","type":"TooManyRequests","param":null,"request_id":"req-1"}`,
			`{"message":"You have exceeded the monthly usage quota.","type":"TooManyRequests","param":"429","code":"AccountQuotaExceeded","status_code":429}`,
			"bad response 429"},
		{"flat_numeric_code", 429,
			`{"code":1302,"message":"rate limited"}`,
			`{"message":"rate limited","type":"upstream_error","param":"429","code":1302,"status_code":429}`,
			"bad response 429"},
		{"anthropic_overloaded", 529,
			`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"},"request_id":"req_011"}`,
			`{"message":"Overloaded","type":"overloaded_error","param":"","code":null,"status_code":529}`,
			"Overloaded"},
		{"anthropic_invalid_request", 400,
			`{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: 9999999 > 64000, which is the maximum allowed number of output tokens for claude-sonnet-4-5"}}`,
			`{"message":"max_tokens: 9999999 \u003e 64000, which is the maximum allowed number of output tokens for claude-sonnet-4-5","type":"invalid_request_error","param":"","code":null,"status_code":400}`,
			"max_tokens: 9999999 > 64000, which is the maximum allowed number of output tokens for claude-sonnet-4-5"},
		{"google_resource_exhausted", 429,
			`{"error":{"code":429,"message":"Resource has been exhausted (e.g. check quota).","status":"RESOURCE_EXHAUSTED"}}`,
			`{"message":"Resource has been exhausted (e.g. check quota).","type":"","param":"","code":429,"status_code":429}`,
			"Resource has been exhausted (e.g. check quota)."},
		{"google_with_details", 400,
			`{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID","domain":"googleapis.com"}]}}`,
			`{"message":"API key not valid. Please pass a valid API key.","type":"","param":"","code":400,"status_code":400}`,
			"API key not valid. Please pass a valid API key."},
		{"legacy_error_string", 404,
			`{"error":"model not found"}`,
			`{"message":"model not found","type":"upstream_error","param":"404","code":"bad_response_status_code","status_code":404}`,
			"bad response 404"},
		{"legacy_msg", 500,
			`{"msg":"internal capacity"}`,
			`{"message":"internal capacity","type":"upstream_error","param":"500","code":"bad_response_status_code","status_code":500}`,
			"bad response 500"},
		{"chained_one_api", 403,
			`{"error":{"message":"user quota is not enough","type":"one_api_error","code":"insufficient_user_quota"}}`,
			`{"message":"user quota is not enough","type":"one_api_error","param":"","code":"insufficient_user_quota","status_code":403}`,
			"user quota is not enough"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RelayErrorHandler(&http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(tc.body))})
			clientJSON, err := json.Marshal(got)
			require.NoError(t, err)
			require.Equal(t, tc.clientJSON, string(clientJSON))
			require.Equal(t, tc.status, got.StatusCode)
			require.True(t, got.FromUpstream)
			require.False(t, got.IsOneAPIError(), "a provider body never describes this gateway's own error")
			require.False(t, got.MessageContainsRawBody)
			require.EqualError(t, got.RawError, tc.rawError)
		})
	}
}
