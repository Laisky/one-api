package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestResponseAPIFormat(t *testing.T) {
	// Create a request similar to the one that caused the error
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "o3",
		Messages: []model.Message{
			{Role: "user", Content: "What is the weather like in Boston?"},
		},
		Stream:      false,
		Temperature: floatPtr(1.0),
		User:        "",
	}

	// Convert to Response API format
	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Marshal to JSON to see the exact format
	jsonData, err := json.Marshal(responseAPI)
	require.NoError(t, err, "Failed to marshal ResponseAPIRequest")

	t.Logf("Generated Response API request: %s", string(jsonData))

	// Verify the input structure
	require.Len(t, responseAPI.Input, 1, "Expected 1 input item")

	// Verify that input[0] is a direct message, not wrapped
	inputMessage, ok := responseAPI.Input[0].(map[string]any)
	require.True(t, ok, "Expected input[0] to be map[string]interface{}, got %T", responseAPI.Input[0])

	// Verify the message has the correct role
	require.Equal(t, "user", inputMessage["role"], "Expected role 'user'")

	// Verify the message has the correct content
	expectedContent := "What is the weather like in Boston?"
	content, ok := inputMessage["content"].([]map[string]any)
	require.True(t, ok && len(content) > 0, "Expected content to be []map[string]interface{}")
	require.Equal(t, expectedContent, content[0]["text"], "Expected correct content text")

	// Parse the JSON back to verify it's valid
	var unmarshaled ResponseAPIRequest
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err, "Failed to unmarshal ResponseAPIRequest")

	// Verify the unmarshaled data matches expectations
	require.Len(t, unmarshaled.Input, 1, "After unmarshal: Expected 1 input item")

	// The unmarshaled input will be map[string]interface{} due to JSON unmarshaling
	inputMap, ok := unmarshaled.Input[0].(map[string]any)
	require.True(t, ok, "After unmarshal: Expected input[0] to be map[string]interface{}, got %T", unmarshaled.Input[0])

	// Verify the role in the map
	role, exists := inputMap["role"]
	require.True(t, exists && role == "user", "After unmarshal: Expected role 'user', got %v", role)

	// Verify the content in the map (should be array format after unmarshaling)
	contentField, exists := inputMap["content"]
	require.True(t, exists, "After unmarshal: Expected content field to exist")
	contentArray, ok := contentField.([]any)
	require.True(t, ok, "After unmarshal: Expected content to be []interface{}, got %T", contentField)
	require.Len(t, contentArray, 1, "After unmarshal: Expected content array length 1")
	contentItem, ok := contentArray[0].(map[string]any)
	require.True(t, ok, "After unmarshal: Expected content[0] to be map[string]interface{}, got %T", contentArray[0])
	require.Equal(t, expectedContent, contentItem["text"], "After unmarshal: Expected correct content text")
}

func TestResponseAPIWithSystemMessage(t *testing.T) {
	// Test the exact scenario from the error log with a system message
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "o3",
		Messages: []model.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "What is the weather like in Boston?"},
		},
		Stream:      false,
		Temperature: floatPtr(1.0),
		User:        "",
	}

	// Convert to Response API format
	responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)

	// Marshal to JSON
	jsonData, err := json.Marshal(responseAPI)
	require.NoError(t, err, "Failed to marshal ResponseAPIRequest")

	t.Logf("Generated Response API request with system message: %s", string(jsonData))

	// Verify system message is moved to instructions
	require.NotNil(t, responseAPI.Instructions, "Expected instructions to be set")
	require.Equal(t, "You are a helpful assistant.", *responseAPI.Instructions, "Expected correct instructions")

	// Verify only user message remains in input
	require.Len(t, responseAPI.Input, 1, "Expected 1 input item after system message removal")

	inputMessage, ok := responseAPI.Input[0].(map[string]any)
	require.True(t, ok, "Expected input[0] to be map[string]interface{}, got %T", responseAPI.Input[0])

	require.Equal(t, "user", inputMessage["role"], "Expected remaining message role 'user'")
}

func TestResponseAPIImageURLFlattening(t *testing.T) {
	// Simulate a Chat Completions message that contains an image_url object
	detail := "high"
	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-4.1-mini",
		Messages: []model.Message{
			{
				Role: "user",
				Content: []model.MessageContent{
					{
						Type: model.ContentTypeText,
						Text: strPtr("请描述这张图片的内容。"),
					},
					{
						Type: model.ContentTypeImageURL,
						ImageURL: &model.ImageURL{
							Url:    "https://example.com/image.jpg",
							Detail: detail,
						},
					},
				},
			},
		},
	}

	resp := ConvertChatCompletionToResponseAPI(chatRequest)
	b, err := json.Marshal(resp)
	require.NoError(t, err, "marshal response")

	// Unmarshal generically to assert structure
	var m map[string]any
	err = json.Unmarshal(b, &m)
	require.NoError(t, err, "unmarshal response")

	input, ok := m["input"].([]any)
	require.True(t, ok && len(input) == 1, "input malformed: %#v", m["input"])
	msg, ok := input[0].(map[string]any)
	require.True(t, ok, "input[0] not object: %T", input[0])
	content, ok := msg["content"].([]any)
	require.True(t, ok && len(content) == 2, "content malformed: %#v", msg["content"])
	// Second item should be input_image with string image_url and preserved detail
	item, ok := content[1].(map[string]any)
	require.True(t, ok, "content[1] not object: %T", content[1])
	require.Equal(t, "input_image", item["type"], "expected type input_image")
	_, isObj := item["image_url"].(map[string]any)
	require.False(t, isObj, "image_url should be string, got object: %#v", item["image_url"])
	urlStr, ok := item["image_url"].(string)
	require.True(t, ok && urlStr != "", "image_url should be non-empty string, got %#v", item["image_url"])
	gotDetail, ok := item["detail"].(string)
	require.True(t, ok && gotDetail == detail, "detail should be preserved as '%s', got %#v", detail, item["detail"])
}

func TestResponseAPIImageDataURLPreserved(t *testing.T) {
	const detail = "low"
	const prefix = "data:image/png;base64,"
	const payload = "QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo="

	chatRequest := &model.GeneralOpenAIRequest{
		Model: "gpt-5-codex",
		Messages: []model.Message{
			{
				Role: "user",
				Content: []model.MessageContent{
					{
						Type: model.ContentTypeText,
						Text: strPtr("Describe the inline image."),
					},
					{
						Type: model.ContentTypeImageURL,
						ImageURL: &model.ImageURL{
							Url:    prefix + payload,
							Detail: detail,
						},
					},
				},
			},
		},
	}

	resp := ConvertChatCompletionToResponseAPI(chatRequest)
	data, err := json.Marshal(resp)
	require.NoError(t, err, "marshal response")

	var parsed map[string]any
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err, "unmarshal response")

	input, ok := parsed["input"].([]any)
	require.True(t, ok && len(input) == 1, "input malformed: %#v", parsed["input"])
	msg, ok := input[0].(map[string]any)
	require.True(t, ok, "input[0] not object: %T", input[0])
	content, ok := msg["content"].([]any)
	require.True(t, ok && len(content) == 2, "content malformed: %#v", msg["content"])
	item, ok := content[1].(map[string]any)
	require.True(t, ok, "content[1] not object: %T", content[1])
	require.Equal(t, "input_image", item["type"], "expected type input_image")
	gotURL, ok := item["image_url"].(string)
	require.True(t, ok, "image_url missing or wrong type: %#v", item["image_url"])
	require.Equal(t, prefix+payload, gotURL, "image_url mismatch")
	detailVal, ok := item["detail"].(string)
	require.True(t, ok && detailVal == detail, "detail should be preserved as '%s', got %#v", detail, item["detail"])

	// Ensure JSON still contains the data URI prefix as documented.
	require.Contains(t, string(data), prefix, "serialized payload should include the data URI prefix")
}

// TestVerbosityConversion tests the verbosity parameter conversion between
// ChatCompletion and Response API formats.
func TestVerbosityConversion(t *testing.T) {
	t.Run("ChatCompletion to Response API with verbosity", func(t *testing.T) {
		verbosityLow := "low"
		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-5",
			Messages: []model.Message{
				{Role: "user", Content: "Hello"},
			},
			Verbosity: &verbosityLow,
			Stream:    true,
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)
		require.NotNil(t, responseAPI.Text, "Expected Text to be set for verbosity conversion")
		require.NotNil(t, responseAPI.Text.Verbosity, "Expected Text.Verbosity to be set")
		require.Equal(t, verbosityLow, *responseAPI.Text.Verbosity, "Expected correct verbosity")

		// Verify JSON serialization
		jsonData, err := json.Marshal(responseAPI)
		require.NoError(t, err, "Failed to marshal")
		require.Contains(t, string(jsonData), `"verbosity":"low"`, "Expected verbosity in JSON")
	})

	t.Run("ChatCompletion to Response API with verbosity and response_format", func(t *testing.T) {
		verbosityMedium := "medium"
		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-5",
			Messages: []model.Message{
				{Role: "user", Content: "What is 2+2?"},
			},
			Verbosity: &verbosityMedium,
			ResponseFormat: &model.ResponseFormat{
				Type: "json_object",
			},
			Stream: false,
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)
		require.NotNil(t, responseAPI.Text, "Expected Text to be set")
		require.NotNil(t, responseAPI.Text.Format, "Expected Text.Format to be set")
		require.Equal(t, "json_object", responseAPI.Text.Format.Type, "Expected format type 'json_object'")
		require.NotNil(t, responseAPI.Text.Verbosity, "Expected Text.Verbosity to be set")
		require.Equal(t, verbosityMedium, *responseAPI.Text.Verbosity, "Expected correct verbosity")
	})

	t.Run("Response API to ChatCompletion with verbosity", func(t *testing.T) {
		verbosityHigh := "high"
		responseAPIRequest := &ResponseAPIRequest{
			Model: "gpt-5",
			Input: ResponseAPIInput{
				map[string]any{
					"role":    "user",
					"content": "Test message",
				},
			},
			Text: &ResponseTextConfig{
				Verbosity: &verbosityHigh,
			},
		}

		chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseAPIRequest)
		require.NoError(t, err, "Conversion failed")
		require.NotNil(t, chatReq.Verbosity, "Expected Verbosity to be set")
		require.Equal(t, verbosityHigh, *chatReq.Verbosity, "Expected correct verbosity")
	})

	t.Run("Response API to ChatCompletion with verbosity and format", func(t *testing.T) {
		verbosityLow := "low"
		responseAPIRequest := &ResponseAPIRequest{
			Model: "gpt-5",
			Input: ResponseAPIInput{
				map[string]any{
					"role":    "user",
					"content": "Test message",
				},
			},
			Text: &ResponseTextConfig{
				Format: &ResponseTextFormat{
					Type: "text",
				},
				Verbosity: &verbosityLow,
			},
		}

		chatReq, err := ConvertResponseAPIToChatCompletionRequest(responseAPIRequest)
		require.NoError(t, err, "Conversion failed")
		require.NotNil(t, chatReq.Verbosity, "Expected Verbosity to be set")
		require.Equal(t, verbosityLow, *chatReq.Verbosity, "Expected correct verbosity")
		require.NotNil(t, chatReq.ResponseFormat, "Expected ResponseFormat to be set")
		require.Equal(t, "text", chatReq.ResponseFormat.Type, "Expected response format type 'text'")
	})

	t.Run("ChatCompletion without verbosity should not set Text", func(t *testing.T) {
		chatRequest := &model.GeneralOpenAIRequest{
			Model: "gpt-5",
			Messages: []model.Message{
				{Role: "user", Content: "Hello"},
			},
			Stream: false,
		}

		responseAPI := ConvertChatCompletionToResponseAPI(chatRequest)
		// Text should only be set if ResponseFormat or Verbosity is specified
		require.True(t, responseAPI.Text == nil || responseAPI.Text.Verbosity == nil, "Text.Verbosity should be nil when not specified in request")
	})
}

func strPtr(s string) *string { return &s }
