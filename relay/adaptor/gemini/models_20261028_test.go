package gemini

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiOctober28NanoBananaGenerateContent verifies the actual native
// request payload is image-capable, strips rejected sampling parameters, and
// preserves native system instructions instead of fabricating chat turns.
func TestGeminiOctober28NanoBananaGenerateContent(t *testing.T) {
	t.Parallel()
	const modelName = "gemini-nano-banana-2.1"
	temp, topP := 0.75, 0.8
	require.Contains(t, ModelList, modelName)
	require.Equal(t, geminiOpenaiCompatible.ModelRatios[modelName], ModelRatios[modelName])
	require.True(t, IsModelSupportSystemInstruction(modelName))

	converted := ConvertRequest(model.GeneralOpenAIRequest{
		Model:       modelName,
		Temperature: &temp,
		TopP:        &topP,
		Messages: []model.Message{
			{Role: "system", Content: "Preserve the subject of the image."},
			{Role: "user", Content: "Create a blue bird illustration."},
		},
	})
	require.Nil(t, converted.GenerationConfig.Temperature)
	require.Nil(t, converted.GenerationConfig.TopP)
	require.Equal(t, []string{"TEXT", "IMAGE"}, converted.GenerationConfig.ResponseModalities)
	require.NotNil(t, converted.SystemInstruction)
	require.Equal(t, "Preserve the subject of the image.", converted.SystemInstruction.Parts[0].Text)
	require.Len(t, converted.Contents, 1)
	require.Equal(t, "user", converted.Contents[0].Role)
	require.Equal(t, "Create a blue bird illustration.", converted.Contents[0].Parts[0].Text)

	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"responseModalities":["TEXT","IMAGE"]`)
	require.NotContains(t, string(encoded), `"temperature"`)
	require.NotContains(t, string(encoded), `"topP"`)

	legacy := ConvertRequest(model.GeneralOpenAIRequest{
		Model:    "gemini-3.1-flash-image",
		Messages: []model.Message{{Role: "user", Content: "Create an image."}},
	})
	require.Nil(t, legacy.GenerationConfig.ResponseModalities, "do not alter numeric-model request compatibility")
}

// TestGeminiOctober28NanoBananaNativeURLs verifies generateContent routes for
// both the default and explicitly selected stable API versions.
func TestGeminiOctober28NanoBananaNativeURLs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stream bool
		action string
	}{
		{false, "generateContent"},
		{true, "streamGenerateContent?alt=sse"},
	} {
		requestMeta := &meta.Meta{
			ActualModelName: "gemini-nano-banana-2.1",
			Mode:            relaymode.ChatCompletions,
			BaseURL:         "https://generativelanguage.googleapis.com",
			IsStream:        tc.stream,
		}
		requestMeta.Config.APIVersion = "v1"
		url, err := (&Adaptor{}).GetRequestURL(requestMeta)
		require.NoError(t, err)
		require.Equal(t, "https://generativelanguage.googleapis.com/v1/models/gemini-nano-banana-2.1:"+tc.action, url)
	}
}

// TestGeminiOctober28NanoBananaImageResponse verifies ordinary inline image
// output remains usable by Chat Completions clients for the named model.
func TestGeminiOctober28NanoBananaImageResponse(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	received := &ChatResponse{
		Candidates: []ChatCandidate{{
			Content: ChatContent{Parts: []Part{
				{Text: "Illustration complete."},
				{InlineData: &InlineData{MimeType: "image/png", Data: "aW1hZ2U="}},
			}},
		}},
	}
	result := responseGeminiChat2OpenAI(ctx, received)
	require.NotNil(t, result)
	require.Len(t, result.Choices, 1)
	parts, ok := result.Choices[0].Message.Content.([]model.MessageContent)
	require.True(t, ok)
	require.Len(t, parts, 2)
	require.NotNil(t, parts[1].ImageURL)
	require.Equal(t, "data:image/png;base64,aW1hZ2U=", parts[1].ImageURL.Url)
	require.Equal(t, 1, countGeminiOutputImages(received).Total)
}

// TestNanoBananaNativeDiscoveryMatchesTransport verifies native discovery only advertises supported inputs.
func TestNanoBananaNativeDiscoveryMatchesTransport(t *testing.T) {
	native := (&Adaptor{}).GetDefaultModelPricing()[geminiOpenaiCompatible.GeminiNanoBanana21Model]
	require.Equal(t, []string{"text", "image"}, native.InputModalities)
	require.Contains(t, native.Description, "native chat")
	require.Contains(t, geminiOpenaiCompatible.ModelRatios[geminiOpenaiCompatible.GeminiNanoBanana21Model].InputModalities, "file")
	require.Equal(t, ModelRatios["gemini-2.5-flash"], (&Adaptor{}).GetDefaultModelPricing()["gemini-2.5-flash"])
}
