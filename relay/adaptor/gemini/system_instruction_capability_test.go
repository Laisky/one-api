package gemini

import (
	"context"
	"testing"

	"github.com/Laisky/one-api/relay/model"

	"github.com/stretchr/testify/require"
)

// TestNativeSystemInstructionCapability keeps unsupported and unknown models on their legacy path.
func TestNativeSystemInstructionCapability(t *testing.T) {
	for _, name := range []string{"gemini-4-hydrogen", "gemini-4-carbon", "gemini-4-carbon-cyber-permissive", "gemini-4-argon-cyber-permissive", "gemini-3.8-flash-cyber", "gemini-4-argon", "gemini-2.5-flash"} {
		require.True(t, IsModelSupportSystemInstruction(name), name)
	}
	for _, name := range []string{"gemini-4-future-unknown", "gemini-omni-1.2-flash", "gemini-3.5-transcribe", "unknown", ""} {
		require.False(t, IsModelSupportSystemInstruction(name), name)
	}
}

// TestNativeSystemInstructionFallbackControl retains ordinary history for unsupported and unknown IDs.
func TestNativeSystemInstructionFallbackControl(t *testing.T) {
	for _, name := range []string{"gemini-4-future-unknown", "gemini-omni-1.2-flash", "unknown"} {
		request := model.GeneralOpenAIRequest{Model: name, Messages: []model.Message{{Role: "system", Content: "Instruction"}, {Role: "user", Content: "Question"}}}
		converted, err := convertRequest(context.Background(), request, false)
		require.NoError(t, err)
		require.Nil(t, converted.SystemInstruction)
		require.Len(t, converted.Contents, 3)
		require.Equal(t, "Instruction", converted.Contents[0].Parts[0].Text)
		require.Equal(t, "model", converted.Contents[1].Role)
		require.Equal(t, "Okay", converted.Contents[1].Parts[0].Text)
		require.Equal(t, "Question", converted.Contents[2].Parts[0].Text)
	}
}
