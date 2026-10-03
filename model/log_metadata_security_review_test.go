package model

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestReviewLogEndpointFailsClosed exercises credential-bearing URL forms rather
// than deriving expectations from the sanitizer under test.
func TestReviewLogEndpointFailsClosed(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"https://user:fixture-secret@example.com/v1/chat?access_token=fixture-secret#fixture-secret", "https://example.com/v1/chat"},
		{"https:fixture-secret@example.com", ""},
		{"https://user:fixture-secret@example.com/%zz?token=fixture-secret", ""},
		{"user:fixture-secret@example.com", ""},
		{"//user:fixture-secret@example.com/path?credential=fixture-secret", "//example.com/path"},
		{"/v1/chat?unknown_credential=fixture-secret", "/v1/chat"},
		{"https://example.com/path?credential=fixture-secret;broken=1", "https://example.com/path"},
		{"https://[::1]:8443/path?#fixture-secret", "https://[::1]:8443/path"},
		{"wss://user:fixture-secret@example.com/v1/realtime?key=fixture-secret", "wss://example.com/v1/realtime"},
		{"mailto:user:fixture-secret@example.com", ""},
		{"https:///fixture-secret", ""},
		{"  ", ""},
	} {
		got := SanitizeLogUpstreamEndpoint(tc.raw)
		require.Equal(t, tc.want, got)
		require.NotContains(t, got, "fixture-secret")
		require.Equal(t, got, SanitizeLogUpstreamEndpoint(got), "sanitization must be idempotent")
	}
}

// TestReviewHistoricalLogSerialization verifies real database decoding followed
// by all external DTO and JSON/persistence boundaries, without mutating the row.
func TestReviewHistoricalLogSerialization(t *testing.T) {
	const raw = `{"upstream_endpoint":"https://operator:fixture-secret@internal.example/v1/chat?token=fixture-secret","cache_write_tokens":{"ephemeral_5m":12},"other":"preserved"}`
	var metadata LogMetadata
	require.NoError(t, metadata.Scan(raw))
	original := metadata[LogMetadataKeyUpstreamEndpoint]
	row := &Log{Metadata: metadata, Quota: 123, PromptTokens: 17, CompletionTokens: 19}
	payload, err := json.Marshal(LogsToResponses([]*Log{row}))
	require.NoError(t, err)
	require.NotContains(t, string(payload), "fixture-secret")
	require.NotContains(t, string(payload), "internal.example")
	require.NotContains(t, string(payload), "upstream_endpoint")
	require.Contains(t, string(payload), `"quota":123`)
	require.Contains(t, string(payload), `"ephemeral_5m":12`)
	persisted, err := metadata.Value()
	require.NoError(t, err)
	require.NotContains(t, persisted.(string), "fixture-secret")
	direct, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NotContains(t, string(direct), "fixture-secret")
	require.Equal(t, original, metadata[LogMetadataKeyUpstreamEndpoint], "never mutate cached or caller-owned metadata")
	for _, invalid := range []any{map[string]any{"key": "fixture-secret"}, []any{"fixture-secret"}, 123, nil} {
		data := LogMetadata{LogMetadataKeyUpstreamEndpoint: invalid, "other": "preserved"}
		b, err := json.Marshal((&Log{Metadata: data}).ToResponse())
		require.NoError(t, err)
		require.NotContains(t, string(b), "upstream_endpoint")
		v, err := data.Value()
		require.NoError(t, err)
		require.NotContains(t, v.(string), "upstream_endpoint")
	}
}
