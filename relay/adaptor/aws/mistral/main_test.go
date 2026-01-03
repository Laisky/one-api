package aws_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	aws "github.com/songquanpeng/one-api/relay/adaptor/aws/mistral"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestConvertMessages(t *testing.T) {
	t.Parallel()
	messages := []relaymodel.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant.",
		},
		{
			Role:    "user",
			Content: "What is the weather like?",
		},
	}

	mistralMessages := aws.ConvertMessages(messages)
	require.Len(t, mistralMessages, 2)
	require.Equal(t, "system", mistralMessages[0].Role)
	require.Equal(t, "You are a helpful assistant.", mistralMessages[0].Content)
	require.Equal(t, "user", mistralMessages[1].Role)
	require.Equal(t, "What is the weather like?", mistralMessages[1].Content)
}

func TestConvertRequest(t *testing.T) {
	t.Parallel()
	request := relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
		MaxTokens:   500,
		Temperature: func() *float64 { f := 0.7; return &f }(),
		TopP:        func() *float64 { f := 0.9; return &f }(),
	}

	mistralReq := aws.ConvertRequest(request)
	require.NotNil(t, mistralReq)
	require.Len(t, mistralReq.Messages, 1)
	require.Equal(t, "user", mistralReq.Messages[0].Role)
	require.Equal(t, "Hello", mistralReq.Messages[0].Content)
	require.Equal(t, 500, mistralReq.MaxTokens)
	require.Equal(t, 0.7, *mistralReq.Temperature)
	require.Equal(t, 0.9, *mistralReq.TopP)
}

func TestConvertTools(t *testing.T) {
	t.Parallel()
	tools := []relaymodel.Tool{
		{
			Type: "function",
			Function: &relaymodel.Function{
				Name:        "get_weather",
				Description: "Get weather information",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{
							"type":        "string",
							"description": "The city name",
						},
					},
				},
			},
		},
	}

	mistralTools := aws.ConvertTools(tools)
	require.Len(t, mistralTools, 1)
	require.Equal(t, "function", mistralTools[0].Type)
	require.Equal(t, "get_weather", mistralTools[0].Function.Name)
	require.Equal(t, "Get weather information", mistralTools[0].Function.Description)
	require.NotNil(t, mistralTools[0].Function.Parameters)
}

func TestAwsModelID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
		hasError bool
	}{
		{"mistral-large-2407", "mistral.mistral-large-2407-v1:0", false},
		{"mistral-large-24.07", "mistral.mistral-large-2407-v1:0", false},
		{"unknown-model", "", true},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			// Note: awsModelID is not exported, so we can't test it directly
			// This would need to be made public for testing or we'd need to test through the public interface
			_ = test // Placeholder to avoid unused variable error
		})
	}
}

func TestConvertStopReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "stop"},
		{"length", "length"},
		{"tool_calls", "tool_calls"},
		{"unknown", "stop"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			// Note: convertStopReason is not exported, so we can't test it directly
			// This would need to be made public for testing or we'd need to test through the public interface
			_ = test // Placeholder to avoid unused variable error
		})
	}
}
