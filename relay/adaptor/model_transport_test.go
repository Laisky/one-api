package adaptor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestValidateRESTModelTransportRejectsEveryLiveOnlyGoogleModel verifies that
// every currently catalogued Live-only model is rejected for every Google REST
// channel family. Parameters: t is the test handle. Returns: none.
func TestValidateRESTModelTransportRejectsEveryLiveOnlyGoogleModel(t *testing.T) {
	liveModels := []string{
		"gemini-3.8-live",
		"gemini-3.8-live-extended-thinking",
		"gemini-3.1-flash-live-preview",
		"gemini-3.5-live-translate-preview",
		"gemini-3.5-transcribe-live",
	}
	googleRESTChannels := []int{
		channeltype.Gemini,
		channeltype.VertextAI,
		channeltype.GeminiOpenAICompatible,
	}

	for _, channel := range googleRESTChannels {
		for _, model := range liveModels {
			t.Run(fmt.Sprintf("channel_%d/%s", channel, model), func(t *testing.T) {
				err := ValidateRESTModelTransport(&meta.Meta{ChannelType: channel, ActualModelName: model})
				require.ErrorIs(t, err, ErrModelRequiresLiveTransport)
				require.True(t, IsRESTTransportMismatch(err))
			})
		}
	}
}

// TestValidateRESTModelTransportAllowsThirdPartyBridgeAndNormalModels verifies
// that the Google transport restriction neither blocks independent REST bridges
// nor changes ordinary Gemini model routing. Parameters: t is the test handle.
// Returns: none.
func TestValidateRESTModelTransportAllowsThirdPartyBridgeAndNormalModels(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		model       string
	}{
		{name: "openai_compatible_bridge", channelType: channeltype.OpenAICompatible, model: "gemini-3.8-live"},
		{name: "ordinary_gemini", channelType: channeltype.Gemini, model: "gemini-3.8-flash"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, ValidateRESTModelTransport(&meta.Meta{
				ChannelType:     tc.channelType,
				ActualModelName: tc.model,
			}))
		})
	}
}

// TestValidateRESTModelTransportUsesEscapedCallerModelInGuidance verifies that
// remediation links keep the caller's model alias, rather than an operator-only
// mapped upstream ID, and cannot be split by query-string characters. Parameters:
// t is the test handle. Returns: none.
func TestValidateRESTModelTransportUsesEscapedCallerModelInGuidance(t *testing.T) {
	err := ValidateRESTModelTransport(&meta.Meta{
		ChannelType:     channeltype.Gemini,
		OriginModelName: "friendly & alias",
		ActualModelName: "gemini-3.8-live",
	})
	require.ErrorIs(t, err, ErrModelRequiresLiveTransport)
	require.Contains(t, err.Error(), "/v1/realtime?model=friendly+%26+alias")
	require.NotContains(t, err.Error(), "model=friendly & alias")
	require.True(t, strings.Contains(err.Error(), "gemini-3.8-live"))
}
