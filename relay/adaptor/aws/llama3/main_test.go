package aws_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	aws "github.com/songquanpeng/one-api/relay/adaptor/aws/llama3"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestConvertRequest(t *testing.T) {
	t.Parallel()
	// Test basic message conversion
	openaiReq := relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{
				Role:    "user",
				Content: "What's your name?",
			},
		},
		MaxTokens:   100,
		Temperature: &[]float64{0.7}[0],
		TopP:        &[]float64{0.9}[0],
		Stop:        []string{"stop1", "stop2"},
	}

	llamaReq := aws.ConvertRequest(openaiReq)

	require.NotNil(t, llamaReq)
	require.Equal(t, 1, len(llamaReq.Messages))
	require.Equal(t, "user", llamaReq.Messages[0].Role)
	require.Equal(t, "What's your name?", llamaReq.Messages[0].Content)
	require.Equal(t, 100, llamaReq.MaxTokens)
	require.Equal(t, 0.7, *llamaReq.Temperature)
	require.Equal(t, 0.9, *llamaReq.TopP)
	require.Equal(t, []string{"stop1", "stop2"}, llamaReq.Stop)

	// Test multi-message conversation
	multiMessageReq := relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{
				Role:    "system",
				Content: "Your name is L. You are a detective.",
			},
			{
				Role:    "user",
				Content: "What's your name?",
			},
			{
				Role:    "assistant",
				Content: "L",
			},
			{
				Role:    "user",
				Content: "What's your job?",
			},
		},
		MaxTokens: 0, // Should use default
	}

	llamaReq = aws.ConvertRequest(multiMessageReq)

	require.NotNil(t, llamaReq)
	require.Equal(t, 4, len(llamaReq.Messages))
	require.Equal(t, "system", llamaReq.Messages[0].Role)
	require.Equal(t, "user", llamaReq.Messages[1].Role)
	require.Equal(t, "assistant", llamaReq.Messages[2].Role)
	require.Equal(t, "user", llamaReq.Messages[3].Role)
	require.True(t, llamaReq.MaxTokens > 0) // Should use default config value
}
