package controller

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/gemini/tts"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestGeminiSpeechModelCannotControlURL rejects user-controlled path, host, and query fragments.
// Accepted IDs must be reconstructed from constants before entering either URL builder.
func TestGeminiSpeechModelCannotControlURL(t *testing.T) {
	t.Parallel()
	for _, modelName := range []string{
		"https://untrusted.invalid/voice", "//untrusted.invalid/voice",
		"gemini-3.8-flash-tts/../../../../other", "gemini-3.8-flash-tts%2f..%2fother",
		"gemini-3.8-flash-tts?key=private-fixture", "gemini-3.8-flash-tts#fragment",
		"gemini-3.8-flash-tts:streamGenerateContent", " gemini-3.8-flash-tts",
		"gemini-3.8-flash-tts\r\nHost: untrusted.invalid",
	} {
		require.Empty(t, tts.CanonicalModelID(modelName))
		for _, channel := range []int{channeltype.Gemini, channeltype.GeminiOpenAICompatible, channeltype.VertextAI} {
			m := &meta.Meta{ActualModelName: modelName, ChannelType: channel, BaseURL: "https://trusted.test/prefix"}
			target, err := geminiSpeechURL(m, true)
			require.Error(t, err)
			require.Empty(t, target)
			require.NotContains(t, err.Error(), "private-fixture")
		}
	}
	for _, modelName := range []string{"gemini-3.8-flash-tts", "gemini-3.8-flash-lite-tts"} {
		for _, streaming := range []bool{false, true} {
			target, err := geminiSpeechURL(&meta.Meta{ActualModelName: modelName, BaseURL: "https://trusted.test/prefix", ChannelType: channeltype.Gemini}, streaming)
			require.NoError(t, err)
			parsed, err := url.Parse(target)
			require.NoError(t, err)
			require.Equal(t, "trusted.test", parsed.Host)
			require.Equal(t, "https", parsed.Scheme)
			require.Contains(t, parsed.Path, "/prefix/v1beta/models/"+modelName+":")
			require.NotContains(t, parsed.RawQuery, "key")
		}
	}
}
