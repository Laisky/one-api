package controller

import (
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// geminiSpeechTargetPolicy freezes the exact destinations authorized by a selected channel.
// Parameters: m supplies administrator-owned routing settings, not client URL fields.
// Returns: an anchored allowlist for the two supported models and two transport modes.
func geminiSpeechTargetPolicy(m *meta.Meta) (*regexp.Regexp, error) {
	if m == nil || !isGeminiSpeechChannel(m.ChannelType) {
		return nil, errors.New("invalid Gemini speech channel")
	}
	// Do not copy the request metadata wholesale: model aliases, incoming paths,
	// credentials, and previously observed upstream URLs are not policy inputs.
	// Pick scalar config fields and copy the endpoint override so the allowlist
	// cannot change if request conversion subsequently mutates routing metadata.
	routing := &meta.Meta{
		ChannelType: m.ChannelType,
		BaseURL:     m.BaseURL,
		Mode:        relaymode.AudioSpeech,
		Config: model.ChannelConfig{
			Region:            m.Config.Region,
			APIVersion:        m.Config.APIVersion,
			VertexAIProjectID: m.Config.VertexAIProjectID,
			EndpointURLs: map[string]string{
				"audio_speech": m.Config.EndpointURLs["audio_speech"],
			},
		},
	}
	if len(routing.BaseURL) > 4096 || len(routing.Config.EndpointURLs["audio_speech"]) > 4096 ||
		len(routing.Config.VertexAIProjectID) > 4096 || len(routing.Config.Region) > 4096 || len(routing.Config.APIVersion) > 16 {
		return nil, errors.New("Gemini speech route configuration exceeds size limit")
	}
	var destinations []string
	for _, modelID := range []string{"gemini-3.8-flash-tts", "gemini-3.8-flash-lite-tts"} {
		routing.ActualModelName = modelID
		for _, stream := range []bool{false, true} {
			target, err := geminiSpeechURL(routing, stream)
			if err != nil {
				return nil, errors.Wrap(err, "resolve authorized speech destination")
			}
			if len(target) > 4096 {
				return nil, errors.New("Gemini speech destination exceeds size limit")
			}
			// Configuration values are literal destinations, never regex syntax.
			// No wildcard is used for the authority, path, query, or fragment.
			destinations = append(destinations, regexp.QuoteMeta(target))
		}
	}
	policy, err := regexp.Compile("^(?:" + strings.Join(destinations, "|") + ")$")
	if err != nil {
		return nil, errors.Wrap(err, "compile speech destination policy")
	}
	return policy, nil
}
