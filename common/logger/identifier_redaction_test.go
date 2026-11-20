package logger

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
)

// identifierTestKind is a named string type, as provider error types are.
type identifierTestKind string

// identifierFieldMap encodes fields the way a structured sink does.
//
// Parameters:
//   - fields: the zap fields to encode.
//
// Returns:
//   - map[string]any: the encoded key/value pairs.
func identifierFieldMap(fields []zap.Field) map[string]any {
	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}
	return encoder.Fields
}

// requireIdentifierLogged asserts that value is logged unchanged as want.
//
// Parameters:
//   - t: the running test.
//   - value: the untrusted value.
//   - want: the expected logged text.
func requireIdentifierLogged(t *testing.T, value any, want string) {
	t.Helper()
	fields := identifierFieldMap(UntrustedIdentifierFields("error_code", value))
	require.Equal(t, want, fields["error_code"], "value %#v must be logged verbatim", value)
	require.Len(t, fields, 1, "a loggable value adds no redaction marker")
}

// requireIdentifierRedacted asserts that value is replaced by the redaction
// marker and flagged with "error_code_redacted".
//
// Parameters:
//   - t: the running test.
//   - value: the untrusted value.
func requireIdentifierRedacted(t *testing.T, value any) {
	t.Helper()
	fields := identifierFieldMap(UntrustedIdentifierFields("error_code", value))
	require.Equal(t, "[REDACTED]", fields["error_code"], "value %#v must be redacted", value)
	require.Equal(t, true, fields["error_code_redacted"])
	require.Len(t, fields, 2)
}

// TestUntrustedIdentifierFieldsAbsent verifies that nil and blank values add no
// field at all.
func TestUntrustedIdentifierFieldsAbsent(t *testing.T) {
	var nilText *string
	var nilMap map[string]any
	var nilSlice []any
	for _, value := range []any{nil, "", " \t\n", nilText, nilMap, nilSlice} {
		require.Empty(t, UntrustedIdentifierFields("error_code", value), "%#v", value)
	}
}

// TestUntrustedIdentifierFieldsLogsProviderVocabulary pins that the provider
// error vocabulary operators search for is logged verbatim: snake_case and
// CamelCase codes, numeric codes of every Go numeric kind, parameter paths and
// dated model identifiers.
func TestUntrustedIdentifierFieldsLogsProviderVocabulary(t *testing.T) {
	for _, value := range []string{
		"rate_limit_exceeded", "insufficient_quota", "context_length_exceeded", "invalid_request_error",
		"AccountQuotaExceeded", "ModelAccountTpmRateLimitExceeded", "ProvisionedThroughputExceededException",
		"IdempotentParameterMismatchException", "InvalidAuthenticationTokenTenant", "HTTPRequestJSONError",
		"XMLHttpRequestFailed", "InvalidURLOrAPIKeyFormat", "TokenIsTooLargeOrInvalid", "MaxTokensIsTooLargeForModel",
		"IPv6AddressInvalid", "Throttling.RateQuota", "InvalidParameter.UnsupportedModel", "one_api_error",
		"1302", "336003", "40000001", "messages[0].content[1].image_url.url", "input/0-a", "tools[12].function.name",
		"claude-3-5-sonnet-20241022", "claude-opus-4-1-20250805", "gpt-4o-2024-08-06", "Qwen2.5-VL-72B-Instruct",
		"models/gemini-2.0-flash", "v1.2.3", strings.Repeat("ab_", 42) + "ab",
	} {
		requireIdentifierLogged(t, value, value)
	}
	requireIdentifierLogged(t, "  rate_limit_error \n", "rate_limit_error")
	requireIdentifierLogged(t, identifierTestKind("one_api_error"), "one_api_error")
	requireIdentifierLogged(t, json.Number("429"), "429")
	requireIdentifierLogged(t, float64(429), "429")
	requireIdentifierLogged(t, float64(40000001), "40000001")
	requireIdentifierLogged(t, -1.5, "-1.5")
	requireIdentifierLogged(t, float32(2.5), "2.5")
	requireIdentifierLogged(t, 7, "7")
	requireIdentifierLogged(t, int64(-42), "-42")
	requireIdentifierLogged(t, uint8(9), "9")
}

// TestUntrustedIdentifierFieldsRedactsFreeForm verifies that values outside the
// identifier shape (free text, e-mail, URLs, padded base64, over-long values)
// and non-scalar values are redacted.
func TestUntrustedIdentifierFieldsRedactsFreeForm(t *testing.T) {
	text := "plain"
	for _, value := range []any{
		"someone@example.com", "rate limit for internal-account-123", `param"`, "account:internal", "a\nb", "ключ",
		"https://provider.example/v1", "dGhpcyBpcyBhIHNlY3JldA==", "a+b", strings.Repeat("a_", 64) + "a",
		true, map[string]any{"account": "internal-account-123"}, []any{"internal-account-123"}, &text, struct{}{},
	} {
		requireIdentifierRedacted(t, value)
	}
}

// TestUntrustedIdentifierFieldsRedactsSecretShapes verifies that
// identifier-shaped credentials and opaque identifiers are redacted even though
// they match the identifier character set.
func TestUntrustedIdentifierFieldsRedactsSecretShapes(t *testing.T) {
	for _, value := range []any{
		// Credential prefixes, at the start or after a separator.
		"sk-proj-AbCdEf0123456789GhIjKlMn", "sk-ant-api03-abcDEF", "sk-live-SECRETSECRET", "sk_live_SECRETSECRETSECRET",
		"sk-1234", "rk_test_abc", "sess-abcdef", "AIzaSyA-example", "ya29.a0AfH6SM", "AKIAIOSFODNN7EXAMPLE",
		"AKIDexample", "ghp_example", "github_pat_example", "glpat-example", "xoxb-1-2-abc", "hf_example",
		"gsk_example", "xai-example", "r8_example", "pplx-example", "nvapi-example", "invalid_key.sk-live-SECRETSECRET",
		// JWT.
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhY2N0In0.c2lnbmF0dXJlLXNpZ25hdHVyZQ",
		// Hex digests and base62/base64url key bodies.
		"9f86d081884c7d659a2feaa0c55ad015", "key-1a2b3c4d5e6f7a8b", "acct_1Nv0FGQ9RKHgCVdK", "org-AbCd1234EfGh5678IjKl",
		// Letter-only random tokens.
		"bPxRfiCYwKqZtLmNaHsJ", "token.QwErTyUiOpAsDfGhJk", strings.Repeat("abcdefgh", 6),
		// Opaque identifiers.
		"3f2504e0-4f89-41d3-9a0c-0305e82c3301", "account-3F2504E0-4F89-41D3-9A0C-0305E82C3301", "10.0.0.1", "ip-192.168.1.20",
		"123456789012", "account_1234567890", int64(4111111111111111), float64(4111111111111111), uint64(12345678901),
	} {
		requireIdentifierRedacted(t, value)
	}
}

// TestUntrustedIdentifierFieldsLengthBoundary pins the 128-byte bound of the
// identifier shape using vocabulary-shaped words.
func TestUntrustedIdentifierFieldsLengthBoundary(t *testing.T) {
	atLimit := strings.Repeat("quota_", 21) + "ab"
	require.Len(t, atLimit, 128)
	requireIdentifierLogged(t, atLimit, atLimit)
	requireIdentifierRedacted(t, atLimit+"c")
}

// TestUntrustedIdentifierFieldsRandomTokenCoverage pins the documented coverage
// of the secret-shape rules on seeded random key bodies, so a later loosening
// of a threshold is caught: every hex token of 16 or more characters and at
// least 99% of base62 and lowercase-alphanumeric tokens of 32 characters must
// be redacted.
func TestUntrustedIdentifierFieldsRandomTokenCoverage(t *testing.T) {
	random := rand.New(rand.NewPCG(20261119, 629))
	for _, scenario := range []struct {
		alphabet string
		length   int
		minRate  float64
	}{
		{"0123456789abcdef", 16, 1},
		{"0123456789abcdef", 64, 1},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", 32, 0.99},
		{"abcdefghijklmnopqrstuvwxyz0123456789", 32, 0.99},
	} {
		const samples = 2000
		redacted := 0
		for range samples {
			token := make([]byte, scenario.length)
			for i := range token {
				token[i] = scenario.alphabet[random.IntN(len(scenario.alphabet))]
			}
			if !loggableIdentifier(string(token)) {
				redacted++
			}
		}
		rate := float64(redacted) / samples
		require.GreaterOrEqual(t, rate, scenario.minRate, "alphabet %q length %d", scenario.alphabet, scenario.length)
	}
}
