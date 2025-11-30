package openai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestAdaptorIntegration(t *testing.T) {
	// Create a mock request
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4",
		Messages: []model.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello, world!"},
		},
		MaxTokens:   150,
		Temperature: floatPtr(0.8),
		Stream:      false,
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "get_weather",
					Description: "Get current weather",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{
								"type":        "string",
								"description": "City name",
							},
						},
						"required": []string{"location"},
					},
				},
			},
		},
		ToolChoice: "auto",
	}

	// Create Gin context
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	// Create meta context
	testMeta := &meta.Meta{
		Mode:           relaymode.ChatCompletions,
		ChannelType:    1, // OpenAI channel type
		RequestURLPath: "/v1/chat/completions",
		BaseURL:        "https://api.openai.com",
	}
	testMeta.ActualModelName = chatRequest.Model
	c.Set(ctxkey.Meta, testMeta)

	// Create adaptor
	adaptor := &Adaptor{}
	adaptor.Init(testMeta)

	// Test URL generation
	url, err := adaptor.GetRequestURL(testMeta)
	require.NoError(t, err, "GetRequestURL failed")

	expectedURL := "/v1/responses"
	require.True(t, contains(url, expectedURL), "Expected URL to contain %s, got %s", expectedURL, url)

	// Test request conversion
	convertedReq, err := adaptor.ConvertRequest(c, relaymode.ChatCompletions, chatRequest)
	require.NoError(t, err, "ConvertRequest failed")

	// Verify it was converted to Response API request
	responseAPIReq, ok := convertedReq.(*ResponseAPIRequest)
	require.True(t, ok, "Expected ResponseAPIRequest, got %T", convertedReq)

	require.Equal(t, "gpt-4", responseAPIReq.Model, "Model mismatch")
	require.NotNil(t, responseAPIReq.MaxOutputTokens, "MaxOutputTokens should not be nil")
	require.Equal(t, 150, *responseAPIReq.MaxOutputTokens, "MaxOutputTokens mismatch")
	require.NotNil(t, responseAPIReq.Instructions, "Instructions should not be nil")
	require.Equal(t, "You are a helpful assistant.", *responseAPIReq.Instructions, "Instructions mismatch")
	require.Len(t, responseAPIReq.Input, 1, "Expected 1 input message after system removal")
	require.NotNil(t, responseAPIReq.Stream, "Stream should not be nil")
	require.False(t, *responseAPIReq.Stream, "Expected non-streaming request")
	require.NotNil(t, responseAPIReq.Temperature, "Temperature should not be nil")
	require.Equal(t, 0.8, *responseAPIReq.Temperature, "Temperature mismatch")
	require.Len(t, responseAPIReq.Tools, 1, "Expected 1 tool")

	// Verify the request can be marshaled to JSON
	jsonData, err := json.Marshal(responseAPIReq)
	require.NoError(t, err, "Failed to marshal ResponseAPIRequest")

	// Verify it can be unmarshaled back without data loss
	var unmarshaled ResponseAPIRequest
	require.NoError(t, json.Unmarshal(jsonData, &unmarshaled), "Failed to unmarshal ResponseAPIRequest")

	t.Logf("Successfully converted ChatCompletion payload to Response API format")
	t.Logf("JSON: %s", string(jsonData))
}

func TestAdaptorNonChatCompletion(t *testing.T) {
	// Test that non-chat completion requests are not converted
	embeddingRequest := &model.GeneralOpenAIRequest{
		Model: "text-embedding-ada-002",
		Input: "Test text",
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = &http.Request{}

	testMeta := &meta.Meta{
		Mode:        relaymode.Embeddings,
		ChannelType: 1,
	}
	c.Set(ctxkey.Meta, testMeta)

	adaptor := &Adaptor{}
	adaptor.Init(testMeta)

	// Test that non-chat completion requests are not converted
	convertedReq, err := adaptor.ConvertRequest(c, relaymode.Embeddings, embeddingRequest)
	require.NoError(t, err, "ConvertRequest failed")

	// Should return the original request unchanged
	originalReq, ok := convertedReq.(*model.GeneralOpenAIRequest)
	require.True(t, ok, "Expected GeneralOpenAIRequest, got %T", convertedReq)
	require.Equal(t, "text-embedding-ada-002", originalReq.Model, "Expected model unchanged")
}

// Helper functions
func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
