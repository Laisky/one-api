package gemini

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiOctober13ArgonSystemInstructions checks actual Chat Completions and
// Claude Messages conversions, not only the catalogue. Parameter t is the test
// handle; it returns no values.
func TestGeminiOctober13ArgonSystemInstructions(t *testing.T) {
	const modelName = "gemini-4-argon"
	const systemPrompt = "Follow the configured operating policy."
	const userPrompt = "Review the proposed change."
	require.Contains(t, ModelList, modelName)
	require.Equal(t, geminiOpenaiCompatible.ModelRatios[modelName], ModelRatios[modelName])
	require.True(t, IsModelSupportSystemInstruction(modelName))

	t.Run("chat-completions", func(t *testing.T) {
		converted := ConvertRequest(model.GeneralOpenAIRequest{
			Model:     modelName,
			MaxTokens: 65_536,
			Messages: []model.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userPrompt},
			},
		})
		assertOctober13ArgonSystemPrompt(t, converted, systemPrompt, userPrompt)
	})
	t.Run("claude-messages", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, &model.ClaudeRequest{
			Model:     modelName,
			System:    systemPrompt,
			MaxTokens: 65_536,
			Messages: []model.ClaudeMessage{
				{Role: "user", Content: userPrompt},
			},
		})
		require.NoError(t, err)
		request, ok := converted.(*ChatRequest)
		require.True(t, ok)
		assertOctober13ArgonSystemPrompt(t, request, systemPrompt, userPrompt)
	})
	for _, excluded := range []string{"gemini-4-unknown", "gemini-3.8-live", "gemini-embedding-2"} {
		require.False(t, IsModelSupportSystemInstruction(excluded), "no family-wide allowlist expansion")
	}
}

// assertOctober13ArgonSystemPrompt verifies native system placement without
// fabricated conversation turns. Parameters are the test handle, converted
// request, and expected system/user text; it returns no values.
func assertOctober13ArgonSystemPrompt(t *testing.T, request *ChatRequest, system, user string) {
	t.Helper()
	require.NotNil(t, request.SystemInstruction)
	require.Empty(t, request.SystemInstruction.Role)
	require.Len(t, request.SystemInstruction.Parts, 1)
	require.Equal(t, system, request.SystemInstruction.Parts[0].Text)
	require.Len(t, request.Contents, 1)
	require.Equal(t, "user", request.Contents[0].Role)
	require.Len(t, request.Contents[0].Parts, 1)
	require.Equal(t, user, request.Contents[0].Parts[0].Text)
	require.Equal(t, 65_536, request.GenerationConfig.MaxOutputTokens)
	require.Nil(t, request.GenerationConfig.ResponseModalities)
}

// TestGeminiOctober13ArgonRequestURLs checks synchronous and streaming REST
// routes and explicit API-version overrides. Parameter t is the test handle;
// it returns no values and makes no provider requests.
func TestGeminiOctober13ArgonRequestURLs(t *testing.T) {
	for _, tt := range []struct {
		name   string
		stream bool
		action string
	}{
		{"synchronous", false, "generateContent"},
		{"streaming", true, "streamGenerateContent?alt=sse"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requestMeta := &meta.Meta{
				ActualModelName: "gemini-4-argon",
				Mode:            relaymode.ChatCompletions,
				BaseURL:         "https://generativelanguage.googleapis.com",
				IsStream:        tt.stream,
			}
			url, err := (&Adaptor{}).GetRequestURL(requestMeta)
			require.NoError(t, err)
			require.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models/gemini-4-argon:"+tt.action, url)
			requestMeta.Config.APIVersion = "v1"
			url, err = (&Adaptor{}).GetRequestURL(requestMeta)
			require.NoError(t, err)
			require.Equal(t, "https://generativelanguage.googleapis.com/v1/models/gemini-4-argon:"+tt.action, url)
		})
	}
}
