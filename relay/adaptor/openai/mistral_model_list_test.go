package openai

import (
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// TestPR421MistralAudioListings checks every audio alias in the dynamic catalog,
// exported compatibility list and the actual OpenAI-compatible channel listing.
func TestPR421MistralAudioListings(t *testing.T) {
	_, compatible := GetCompatibleChannelMeta(channeltype.Mistral)
	dynamic := (&mistral.Adaptor{}).GetModelList()
	for _, name := range []string{"voxtral-mini-2602", "voxtral-mini-tts-2603", "voxtral-mini-tts-latest"} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, dynamic, name)
			require.Contains(t, mistral.ModelList, name)
			require.Contains(t, compatible, name)
		})
	}
	require.ElementsMatch(t, dynamic, mistral.ModelList)
	require.ElementsMatch(t, dynamic, compatible)
}
