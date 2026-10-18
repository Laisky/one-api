package otelbridge

import (
	"math"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// TestPrivateLogTextPreservesDiagnostics verifies noncatalog dynamic messages
// remain useful while recognized credentials and labeled payloads are removed.
func TestPrivateLogTextPreservesDiagnostics(t *testing.T) {
	cases := []struct{ name, input, expected string }{
		{"metadata boundary", "metadata:ready, token_count=12 password_reset_required=false", "metadata:ready, token_count=12 password_reset_required=false"},
		{"read pipe error", "failed to read response body: io: read/write on closed pipe", "failed to read response body: io: read/write on closed pipe"},
		{"ordinary", "worker shard 7 completed 13 checks", "worker shard 7 completed 13 checks"},
		{"error", "dial tcp cache.internal:6379: connection refused", "dial tcp cache.internal:6379: connection refused"},
		{"auth", "upstream rejected Bearer sentinel-auth-token", "upstream rejected Bearer [REDACTED]"},
		{"key", "upstream rejected sk-synthetic-key", "upstream rejected [REDACTED]"},
		{"assignment", "dial failed password=sentinel-password; retries=3", "dial failed password=[REDACTED]; retries=3"},
		{"url", "Get https://user:sentinel-password@api.example/v1?unknown=sentinel-query#sentinel-fragment: timeout", "Get https://api.example/v1 timeout"},
		{"payload", "decode failed: response body: sentinel-private-prompt", "decode failed: response body: [REDACTED]"},
		{"json", `upstream failed: response body: {"error":{"message":"sentinel-private-prompt"}}`, "upstream failed: response body: [REDACTED]"},
		{"ordinary metadata", `worker metadata {"retries":3}`, `worker metadata {"retries":3}`},
		{"read error with payload", "failed to read response body: provider rejected prompt: SYNTHETIC_PRIVATE_PROMPT", "failed to read response body: provider rejected prompt: [REDACTED]"},
		{"read failure", "read response body: unexpected EOF", "read response body: unexpected EOF"},
		{"quoted", `connect failed password="first \"quoted\" synthetic secret"; retries=3`, "connect failed password=[REDACTED]; retries=3"},
		{"unterminated", `connect failed password="first synthetic secret remainder`, "connect failed password=[REDACTED]"},
		{"versioned", "exchange failed api_key_v2=SYNTHETIC_KEY; retries=3", "exchange failed api_key_v2=[REDACTED]; retries=3"},
		{"data", "decode image data:image/png;base64,SYNTHETIC_IMAGE error", "decode image [REDACTED data URL] error"},
		{"cookie", "Cookie: session=sentinel-cookie; other=sentinel-other", "Cookie: [REDACTED]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeLogText(tc.input)
			require.Equal(t, tc.expected, got)
			require.Equal(t, got, SanitizeLogText(got), "bridge and SDK both sanitize")
		})
	}
	long := SanitizeLogText(strings.Repeat("中", maxPrivateTextBytes))
	require.True(t, utf8.ValidString(long))
	require.LessOrEqual(t, len(long), maxPrivateTextBytes+len(" [truncated]"))
}

// TestPrivateLogScalarFieldsPreserveTypes verifies arbitrary benign scalar keys
// survive without opening object serialization or weakening identity validation.
func TestPrivateLogScalarFieldsPreserveTypes(t *testing.T) {
	for _, original := range []attribute.KeyValue{
		attribute.String("component", "worker-7"), attribute.String("content_type", "application/json"),
		attribute.Int64("content_length", 123), attribute.Int64("prompt_tokens", 17),
		attribute.Int64("sync_frequency", -1), attribute.Int("token_count", 12),
		attribute.Bool("password_reset_required", false), attribute.Float64("query_latency", 0.25), attribute.Bool("ready", true), attribute.Float64("ratio", 0.25),
	} {
		safe, ok := SanitizeLogAttribute(original)
		require.True(t, ok, string(original.Key))
		require.Equal(t, original, safe)
	}
	for _, key := range []string{"Authorization", "API-Key", "request_body", "prompt", "input", "headers"} {
		safe, ok := SanitizeLogAttribute(attribute.String(key, "sentinel-secret"))
		require.True(t, ok)
		require.Equal(t, "[REDACTED]", safe.Value.AsString())
	}
	for _, bad := range []attribute.KeyValue{
		attribute.Float64("ratio", math.NaN()), attribute.Float64("ratio", math.Inf(1)),
		attribute.Int("channel_id", -1), attribute.String("prompt_tokens", "secret"),
		attribute.StringSlice("items", []string{"sentinel-secret"}),
		attribute.Map("object", attribute.String("password", "sentinel-secret")),
		attribute.String("bad\nkey", "sentinel-secret"), attribute.String("service.name", "forged"),
		attribute.String("code.file.path", "forged"), attribute.String("host_name", "forged"),
	} {
		_, ok := SanitizeLogAttribute(bad)
		require.False(t, ok, string(bad.Key))
	}
}

// TestPrivateLogEndpointsRetainRoutes verifies absolute and relative endpoint
// fields lose userinfo, all query/fragment content and inline data payloads.
func TestPrivateLogEndpointsRetainRoutes(t *testing.T) {
	for _, tc := range []struct{ raw, expected string }{
		{"/v1/responses?unknown=SYNTHETIC_QUERY#SYNTHETIC_FRAGMENT", "/v1/responses"},
		{"https://user:SYNTHETIC_PASSWORD@api.example/v1?x=SYNTHETIC_QUERY#fragment", "https://api.example/v1"},
		{"data:image/png;base64,SYNTHETIC_IMAGE", "[REDACTED data URL]"},
		{"https://invalid%zz/SYNTHETIC_SECRET", "[REDACTED URL]"},
		{"/token/SYNTHETIC_PATH/inspect", "/token/REDACTED/inspect"},
		{"/v1/sk-synthetic-key", "/v1/REDACTED"},
	} {
		safe, ok := SanitizeLogAttribute(attribute.String("url", tc.raw))
		require.True(t, ok)
		require.Equal(t, tc.expected, safe.Value.AsString())
	}
}

// TestPrivateLogUnterminatedSecretAtByteLimit verifies a quote cut by the
// sanitizer budget never turns a secret suffix into apparently ordinary text.
func TestPrivateLogUnterminatedSecretAtByteLimit(t *testing.T) {
	raw := `connection failed password="` + strings.Repeat("SYNTHETIC SECRET ", 1000)
	safe := SanitizeLogText(raw)
	require.Equal(t, "connection failed password=[REDACTED]", safe)
}

// TestPrivateLogCredentialTextAliases verifies recognized secret field names are
// protected in message/error text too, without suppressing the ordinary context.
func TestPrivateLogCredentialTextAliases(t *testing.T) {
	for _, name := range []string{"private_key", "secret_key", "password_hash", "session_secret"} {
		t.Run(name, func(t *testing.T) {
			text := `config failed: {"` + name + `":"SYNTHETIC_PRIVATE_VALUE"}; retries=3`
			safe := SanitizeLogText(text)
			require.Contains(t, safe, "config failed")
			require.Contains(t, safe, "retries=3")
			require.NotContains(t, safe, "SYNTHETIC_PRIVATE_VALUE")
		})
	}
}
