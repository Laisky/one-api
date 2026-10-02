package gemini

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/model"
)

// TestGeminiSeptember18SystemInstructions preserves system prompts for catalog IDs.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember18SystemInstructions(t *testing.T) {
	t.Parallel()
	for _, modelName := range []string{
		"gemini-3-pro-image",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-lite-image",
		"gemini-robotics-er-2-preview",
		"gemini-2.5-computer-use-preview-10-2025",
		"gemini-3.8-flash",
	} {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, ModelList, modelName)
			require.True(t, IsModelSupportSystemInstruction(modelName))
			const systemPrompt = "Follow the configured operating policy."
			const userPrompt = "Summarize the next task."

			t.Run("chat_completions", func(t *testing.T) {
				request := ConvertRequest(model.GeneralOpenAIRequest{
					Model: modelName,
					Messages: []model.Message{
						{Role: "system", Content: systemPrompt},
						{Role: "user", Content: userPrompt},
					},
				})
				assertSeptember18SystemPrompt(t, request, systemPrompt, userPrompt)
			})
			t.Run("claude_messages", func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, &model.ClaudeRequest{
					Model:     modelName,
					System:    systemPrompt,
					MaxTokens: 256,
					Messages: []model.ClaudeMessage{
						{Role: "user", Content: userPrompt},
					},
				})
				require.NoError(t, err)
				request, ok := converted.(*ChatRequest)
				require.True(t, ok)
				assertSeptember18SystemPrompt(t, request, systemPrompt, userPrompt)
			})
		})
	}
}

// assertSeptember18SystemPrompt checks native placement and the absence of synthetic turns.
// Parameters: t is the test handle, request is converted payload, and system/user are expected text.
// Returns: none.
func assertSeptember18SystemPrompt(t *testing.T, request *ChatRequest, system string, user string) {
	t.Helper()
	require.NotNil(t, request.SystemInstruction)
	require.Empty(t, request.SystemInstruction.Role)
	require.Len(t, request.SystemInstruction.Parts, 1)
	require.Equal(t, system, request.SystemInstruction.Parts[0].Text)
	require.Len(t, request.Contents, 1)
	require.Equal(t, "user", request.Contents[0].Role)
	require.Len(t, request.Contents[0].Parts, 1)
	require.Equal(t, user, request.Contents[0].Parts[0].Text)
}

// TestGeminiSeptember18SharedCatalog verifies dependency initialization exposes Live metadata.
// Parameters: t is the current test handle. Returns: none.
func TestGeminiSeptember18SharedCatalog(t *testing.T) {
	t.Parallel()
	for _, modelName := range []string{"gemini-3.8-live", "gemini-3.8-live-extended-thinking"} {
		require.Contains(t, ModelList, modelName)
		require.Equal(t, geminiOpenaiCompatible.ModelRatios[modelName], ModelRatios[modelName])
		// Live catalog metadata must not expand the REST system-instruction allowlist.
		require.False(t, IsModelSupportSystemInstruction(modelName))
	}
	for _, modelName := range []string{"gemini-embedding-2", "gemma-3-27b-it", "gemini-99-unknown"} {
		require.False(t, IsModelSupportSystemInstruction(modelName))
	}
}
