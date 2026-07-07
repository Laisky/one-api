package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

func realtimeMeta(base, override, actualModel string) *meta.Meta {
	m := &meta.Meta{
		Mode:            relaymode.Realtime,
		BaseURL:         base,
		ActualModelName: actualModel,
	}
	if override != "" {
		m.Config = model.ChannelConfig{EndpointURLs: map[string]string{"realtime": override}}
	}
	return m
}

// TestRealtimeWebSocketUpstreamURL verifies the realtime WebSocket connect URL
// honors a per-endpoint "realtime" override (full host+path), normalizes the
// scheme to ws/wss, and applies the mapped model query.
func TestRealtimeWebSocketUpstreamURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		base        string
		override    string
		actualModel string
		clientQuery string
		want        string
	}{
		{
			name: "empty base falls back to openai",
			want: "wss://api.openai.com/v1/realtime",
		},
		{
			name: "https base without override uses canonical path",
			base: "https://api.example.com",
			want: "wss://api.example.com/v1/realtime",
		},
		{
			name:        "model query applied and preserved",
			base:        "https://api.example.com",
			actualModel: "gpt-4o-realtime",
			want:        "wss://api.example.com/v1/realtime?model=gpt-4o-realtime",
		},
		{
			name: "http base preserved as insecure ws",
			base: "http://localhost:8080",
			want: "ws://localhost:8080/v1/realtime",
		},
		{
			name:     "override with full path is respected",
			base:     "https://api.example.com",
			override: "https://custom-host/custom/realtime",
			want:     "wss://custom-host/custom/realtime",
		},
		{
			name:     "override host only uses canonical path",
			base:     "https://api.example.com",
			override: "https://custom-host",
			want:     "wss://custom-host/v1/realtime",
		},
		{
			name:     "override with wss scheme preserved",
			base:     "https://api.example.com",
			override: "wss://custom-host/rt",
			want:     "wss://custom-host/rt",
		},
		{
			name:        "override plus model query",
			base:        "https://api.example.com",
			override:    "https://custom-host/rt",
			actualModel: "gpt-4o-realtime",
			want:        "wss://custom-host/rt?model=gpt-4o-realtime",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := realtimeWebSocketUpstreamURL(realtimeMeta(tc.base, tc.override, tc.actualModel), tc.clientQuery)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestRealtimeSessionsUpstreamURL verifies the realtime sessions (ephemeral token)
// URL keeps the protocol-fixed /v1/realtime/sessions path while routing to the
// override's host when a "realtime" override is set.
func TestRealtimeSessionsUpstreamURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		base     string
		override string
		want     string
	}{
		{
			name: "empty base falls back to openai",
			want: "https://api.openai.com/v1/realtime/sessions",
		},
		{
			name: "https base without override",
			base: "https://api.example.com",
			want: "https://api.example.com/v1/realtime/sessions",
		},
		{
			name:     "override routes to override host, path preserved",
			base:     "https://api.example.com",
			override: "https://custom-host/custom/realtime",
			want:     "https://custom-host/v1/realtime/sessions",
		},
		{
			name:     "override host only",
			base:     "https://api.example.com",
			override: "https://custom-host",
			want:     "https://custom-host/v1/realtime/sessions",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := realtimeSessionsUpstreamURL(realtimeMeta(tc.base, tc.override, ""))
			require.Equal(t, tc.want, got)
		})
	}
}
