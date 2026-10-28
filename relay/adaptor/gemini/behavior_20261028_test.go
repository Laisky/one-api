package gemini

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestStreamPreservesMultipleParts verifies all stream content reaches clients in order.
func TestStreamPreservesMultipleParts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	response := &ChatResponse{Candidates: []ChatCandidate{{Content: ChatContent{Parts: []Part{
		{Text: "before"}, {InlineData: &InlineData{MimeType: "image/png", Data: "YQ=="}},
		{FileData: &FileData{MimeType: "image/png", FileURI: "https://example.test/b.png"}}, {Text: "after"},
	}}}}}
	chunk := streamResponseGeminiChat2OpenAI(c, response)
	require.NotNil(t, chunk)
	wire, err := json.Marshal(chunk)
	require.NoError(t, err)
	var parsed struct {
		Choices []struct {
			Delta struct {
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(wire, &parsed))
	require.Len(t, parsed.Choices, 1)
	parts := parsed.Choices[0].Delta.Content
	require.Len(t, parts, 4)
	require.Equal(t, "before", parts[0].Text)
	require.Equal(t, "data:image/png;base64,YQ==", parts[1].ImageURL.URL)
	require.Equal(t, "https://example.test/b.png", parts[2].ImageURL.URL)
	require.Equal(t, "after", parts[3].Text)
	require.Equal(t, 2, countGeminiOutputImages(response).Total)
}

// TestNanoBananaSystemInstructionCase verifies case variants preserve system instructions.
func TestNanoBananaSystemInstructionCase(t *testing.T) {
	require.False(t, IsModelSupportSystemInstruction("Gemini-2.5-flash"), "preserve legacy case-sensitive lookup")
	for _, name := range []string{"gemini-nano-banana-2.1", "Gemini-Nano-Banana-2.1", "gemini-2.5-flash"} {
		t.Run(name, func(t *testing.T) {
			converted := ConvertRequest(model.GeneralOpenAIRequest{Model: name, Messages: []model.Message{{Role: "system", Content: "instruction"}, {Role: "user", Content: "hello"}}})
			require.NotNil(t, converted.SystemInstruction)
			require.Equal(t, "instruction", converted.SystemInstruction.Parts[0].Text)
			require.Len(t, converted.Contents, 1)
		})
	}
}

// TestNanoBananaNativeRequestCapabilities verifies explicit effort, unsupported file and image limits.
func TestNanoBananaNativeRequestCapabilities(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	for _, effort := range []string{"minimal", "medium", "high"} {
		req := &model.GeneralOpenAIRequest{Model: "gemini-nano-banana-2.1", ReasoningEffort: &effort, Messages: []model.Message{{Role: "user", Content: "draw"}}}
		got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
		require.NoError(t, err)
		raw, err := json.Marshal(got)
		require.NoError(t, err)
		require.Contains(t, string(raw), `"thinkingLevel":"`+effort+`"`)
	}
	var fileReq model.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-nano-banana-2.1","messages":[{"role":"user","content":[{"type":"file","file_data":"YWJj","filename":"a.pdf"}]}]}`), &fileReq))
	_, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &fileReq)
	require.Error(t, err)
	choiceReq := &model.GeneralOpenAIRequest{Model: "gemini-nano-banana-2.1", ToolChoice: map[string]any{"type": "function", "function": map[string]any{"name": "x"}}, Messages: []model.Message{{Role: "user", Content: "draw"}}}
	_, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, choiceReq)
	require.Error(t, err)
	choiceReq.ToolChoice = "none"
	_, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, choiceReq)
	require.NoError(t, err)
}

// TestNanoBananaReferenceLimit verifies the limit applies across the entire request.
func TestNanoBananaReferenceLimit(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	image := model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: "data:image/png;base64,YQ=="}}
	req := &model.GeneralOpenAIRequest{Model: "gemini-nano-banana-2.1", Messages: []model.Message{{Role: "user", Content: []model.MessageContent{image}}}}
	for i := 1; i < 14; i++ {
		req.Messages = append(req.Messages, model.Message{Role: "user", Content: []model.MessageContent{image}})
	}
	got, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	req.Messages = append(req.Messages, model.Message{Role: "user", Content: []model.MessageContent{image}})
	_, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, req)
	require.Error(t, err)
}
