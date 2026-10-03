package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiSpeechTargetPolicy admits only exact channel destinations and preserves proxies.
// Parameters: t is the test handle. Returns: none.
func TestGeminiSpeechTargetPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		channel int
		base    string
		config  model.ChannelConfig
	}{
		{name: "developer_default", channel: channeltype.Gemini},
		{name: "compatible_default", channel: channeltype.GeminiOpenAICompatible},
		{name: "private_proxy_prefix", channel: channeltype.GeminiOpenAICompatible, base: "http://127.0.0.1:8080/proxy/a+b/v1beta/openai"},
		{name: "version_override", channel: channeltype.Gemini, base: "https://proxy.example.test/root", config: model.ChannelConfig{APIVersion: "v1alpha"}},
		{name: "vertex_project", channel: channeltype.VertextAI, config: model.ChannelConfig{VertexAIProjectID: "configured-project"}},
		{name: "vertex_private_proxy", channel: channeltype.VertextAI, base: "https://private.example.test", config: model.ChannelConfig{VertexAIProjectID: "configured-project"}},
		{name: "endpoint_override", channel: channeltype.Gemini, config: model.ChannelConfig{EndpointURLs: map[string]string{"audio_speech": "https://speech.example.test/custom+endpoint"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &meta.Meta{ChannelType: tc.channel, BaseURL: tc.base, Config: tc.config, Mode: relaymode.AudioSpeech}
			policy, err := geminiSpeechTargetPolicy(m)
			require.NoError(t, err)
			for _, modelID := range []string{"gemini-3.8-flash-tts", "gemini-3.8-flash-lite-tts"} {
				m.ActualModelName = modelID
				for _, stream := range []bool{false, true} {
					target, err := geminiSpeechURL(m, stream)
					require.NoError(t, err)
					require.True(t, policy.MatchString(target), target)
					for _, rejected := range []string{
						"https://attacker.invalid/", "http://169.254.169.254/latest/meta-data/",
						"https://attacker.invalid@" + strings.TrimPrefix(target, "https://"),
						target + "/../admin", target + "?key=secret", target + "&key=secret",
						target + "#fragment", target + "\r\nX-Injected: true", target + "\n",
						strings.ReplaceAll(target, ".", "X"),
					} {
						if rejected != target {
							require.False(t, policy.MatchString(rejected), rejected)
						}
					}
				}
			}
		})
	}
}

// TestGeminiSpeechTargetPolicyIsIndependentOfRequestFields checks that request data cannot widen policy.
// Parameters: t is the test handle. Returns: none.
func TestGeminiSpeechTargetPolicyIsIndependentOfRequestFields(t *testing.T) {
	t.Parallel()
	m := &meta.Meta{
		ChannelType: channeltype.Gemini, BaseURL: "https://configured.example.test/root",
		Mode: relaymode.AudioSpeech, ActualModelName: "gemini-3.8-flash-tts",
		Config: model.ChannelConfig{EndpointURLs: map[string]string{"audio_speech": "https://configured.example.test/speech"}},
	}
	policy, err := geminiSpeechTargetPolicy(m)
	require.NoError(t, err)
	m.OriginModelName = "https://attacker.invalid/#"
	m.ActualModelName = "../../admin?key=secret"
	m.RequestURLPath = "https://attacker.invalid/"
	m.UpstreamRequestURL = "https://attacker.invalid/"
	m.APIKey = "private-fixture-key"
	same, err := geminiSpeechTargetPolicy(m)
	require.NoError(t, err)
	require.Equal(t, policy.String(), same.String())
	// A later mutation of the metadata/config map cannot mutate an already
	// constructed policy. A different channel requires a fresh per-attempt policy.
	m.BaseURL = "https://attacker.invalid/"
	m.Config.EndpointURLs["audio_speech"] = "https://attacker.invalid/"
	require.False(t, policy.MatchString(m.Config.EndpointURLs["audio_speech"]))
	require.True(t, policy.MatchString("https://configured.example.test/speech"))
	require.NotContains(t, policy.String(), "private-fixture-key")
	require.NotContains(t, policy.String(), "attacker")
}

// TestGeminiSpeechTargetPolicyRejectsInvalidConfig fails closed on malformed or oversized routes.
// Parameters: t is the test handle. Returns: none.
func TestGeminiSpeechTargetPolicyRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	for _, m := range []*meta.Meta{
		nil,
		{ChannelType: channeltype.OpenAI},
		{ChannelType: channeltype.Gemini, BaseURL: "https://user:secret@proxy.example.test"},
		{ChannelType: channeltype.Gemini, BaseURL: "https://proxy.example.test?bad=%zz"},
		{ChannelType: channeltype.Gemini, BaseURL: "https://proxy.example.test?key=secret"},
		{ChannelType: channeltype.Gemini, BaseURL: "https://proxy.example.test#fragment"},
		{ChannelType: channeltype.Gemini, BaseURL: "https://proxy.example.test/" + strings.Repeat("x", 4096)},
		{ChannelType: channeltype.Gemini, Config: model.ChannelConfig{APIVersion: "../admin"}},
		{ChannelType: channeltype.VertextAI},
	} {
		policy, err := geminiSpeechTargetPolicy(m)
		require.Error(t, err)
		require.Nil(t, policy)
	}
}
