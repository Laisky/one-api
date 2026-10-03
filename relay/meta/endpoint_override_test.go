package meta

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestUpstreamEndpointURLOverride verifies that per-endpoint upstream URL
// overrides are resolved against the request's relay mode, and that missing,
// blank, or mode-mismatched entries fall back to "" (use the default URL).
func TestUpstreamEndpointURLOverride(t *testing.T) {
	t.Parallel()

	t.Run("nil meta returns empty", func(t *testing.T) {
		var m *Meta
		require.Equal(t, "", m.UpstreamEndpointURLOverride())
	})

	t.Run("no config returns empty", func(t *testing.T) {
		m := &Meta{Mode: relaymode.Rerank}
		require.Equal(t, "", m.UpstreamEndpointURLOverride())
	})

	t.Run("matching endpoint returns override", func(t *testing.T) {
		m := &Meta{
			Mode: relaymode.Rerank,
			Config: model.ChannelConfig{
				EndpointURLs: map[string]string{
					"rerank": "https://custom.example.com/v2/rerank",
				},
			},
		}
		require.Equal(t, "https://custom.example.com/v2/rerank", m.UpstreamEndpointURLOverride())
	})

	t.Run("override is trimmed", func(t *testing.T) {
		m := &Meta{
			Mode: relaymode.Rerank,
			Config: model.ChannelConfig{
				EndpointURLs: map[string]string{
					"rerank": "  https://custom.example.com/v2/rerank  ",
				},
			},
		}
		require.Equal(t, "https://custom.example.com/v2/rerank", m.UpstreamEndpointURLOverride())
	})

	t.Run("non-matching endpoint mode returns empty", func(t *testing.T) {
		m := &Meta{
			Mode: relaymode.ChatCompletions,
			Config: model.ChannelConfig{
				EndpointURLs: map[string]string{
					"rerank": "https://custom.example.com/v2/rerank",
				},
			},
		}
		require.Equal(t, "", m.UpstreamEndpointURLOverride())
	})

	t.Run("blank override for matching endpoint returns empty", func(t *testing.T) {
		m := &Meta{
			Mode: relaymode.Rerank,
			Config: model.ChannelConfig{
				EndpointURLs: map[string]string{
					"rerank": "   ",
				},
			},
		}
		require.Equal(t, "", m.UpstreamEndpointURLOverride())
	})

	t.Run("unknown relay mode returns empty", func(t *testing.T) {
		m := &Meta{
			Mode: relaymode.Unknown,
			Config: model.ChannelConfig{
				EndpointURLs: map[string]string{
					"chat_completions": "https://custom.example.com/v1/chat/completions",
				},
			},
		}
		require.Equal(t, "", m.UpstreamEndpointURLOverride())
	})
}
