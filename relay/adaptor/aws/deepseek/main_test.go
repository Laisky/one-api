package aws_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	aws "github.com/songquanpeng/one-api/relay/adaptor/aws/deepseek"
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

	deepseekMessages := aws.ConvertMessages(messages)
	require.Len(t, deepseekMessages, 2)
	require.Equal(t, "system", deepseekMessages[0].Role)
	require.Equal(t, "You are a helpful assistant.", deepseekMessages[0].Content)
	require.Equal(t, "user", deepseekMessages[1].Role)
	require.Equal(t, "What is the weather like?", deepseekMessages[1].Content)
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
		Stop:        []string{"END"},
	}

	deepseekReq := aws.ConvertRequest(request)
	require.NotNil(t, deepseekReq)
	require.Len(t, deepseekReq.Messages, 1)
	require.Equal(t, "user", deepseekReq.Messages[0].Role)
	require.Equal(t, "Hello", deepseekReq.Messages[0].Content)
	require.Equal(t, 500, deepseekReq.MaxTokens)
	require.Equal(t, 0.7, *deepseekReq.Temperature)
	require.Equal(t, 0.9, *deepseekReq.TopP)
	require.Equal(t, []string{"END"}, deepseekReq.Stop)
}

func TestConvertRequestWithStopSequence(t *testing.T) {
	t.Parallel()
	// Test with stop as []interface{} (typical API input)
	request := relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
		Stop: []any{"STOP", "END"},
	}

	deepseekReq := aws.ConvertRequest(request)
	require.NotNil(t, deepseekReq)
	require.Equal(t, []string{"STOP", "END"}, deepseekReq.Stop)
}

func TestConvertRequestWithStopString(t *testing.T) {
	t.Parallel()
	// Test with stop as string
	request := relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
		Stop: "STOP",
	}

	deepseekReq := aws.ConvertRequest(request)
	require.NotNil(t, deepseekReq)
	require.Equal(t, []string{"STOP"}, deepseekReq.Stop)
}

func TestAwsModelID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		expected string
		hasError bool
	}{
		{"deepseek-r1", "deepseek.r1-v1:0", false},
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
		{"end_turn", "stop"},
		{"length", "length"},
		{"max_tokens", "length"},
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
