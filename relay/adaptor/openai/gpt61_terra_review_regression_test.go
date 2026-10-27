package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT61TerraResponseInstructionPreservation verifies that GPT-6 Responses
// conversion does not lose the caller's first system message. It checks the
// final serialized provider request, not just an intermediate message slice.
func TestGPT61TerraResponseInstructionPreservation(t *testing.T) {
	for _, name := range []string{
		"gpt-6.1-terra", "gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna", "gpt-6-astra",
	} {
		t.Run(name, func(t *testing.T) {
			a := &Adaptor{ChannelType: channeltype.OpenAI}
			info := &meta.Meta{
				ChannelType:     channeltype.OpenAI,
				ActualModelName: name,
				Mode:            relaymode.ChatCompletions,
			}
			effort := "high"
			request := &model.GeneralOpenAIRequest{
				Model:           name,
				ReasoningEffort: &effort,
				Messages: []model.Message{
					{Role: "system", Content: "Preserve this system instruction."},
					{Role: "user", Content: "Answer the question."},
				},
			}

			require.True(t, shouldForceResponseAPI(info))
			require.NoError(t, a.applyRequestTransformations(info, request))
			converted := ConvertChatCompletionToResponseAPI(request)
			require.NotNil(t, converted.Instructions)
			require.Equal(t, "Preserve this system instruction.", *converted.Instructions)
			require.Len(t, converted.Input, 1)
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &payload))
			require.Equal(t, `"Preserve this system instruction."`, string(payload["instructions"]))
			if name == "gpt-6.1-terra" {
				require.Equal(t, `"default"`, string(payload["service_tier"]))
			}
		})
	}
}

// TestGPT61TerraLegacyReasoningInstructionCompatibility pins the existing
// legacy o1 Chat policy while newer native GPT-6 Responses preserves instructions.
func TestGPT61TerraLegacyReasoningInstructionCompatibility(t *testing.T) {
	a := &Adaptor{ChannelType: channeltype.OpenAI}
	info := &meta.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "o1-preview",
		Mode:            relaymode.ChatCompletions,
	}
	request := &model.GeneralOpenAIRequest{
		Model:    "o1-preview",
		Messages: []model.Message{
			{Role: "system", Content: "Legacy instruction."},
			{Role: "user", Content: "Hello."},
		},
	}
	require.NoError(t, a.applyRequestTransformations(info, request))
	require.Len(t, request.Messages, 1)
	require.Equal(t, "user", request.Messages[0].Role)
}
