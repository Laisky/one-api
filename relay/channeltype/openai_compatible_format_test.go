package channeltype

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAICompatibleAPIFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty defaults to chat", "", OpenAICompatibleAPIFormatChatCompletion},
		{"whitespace defaults", "   ", OpenAICompatibleAPIFormatChatCompletion},
		{"chat-completion alias", "Chat-Completion", OpenAICompatibleAPIFormatChatCompletion},
		{"chat shorthand", "chat", OpenAICompatibleAPIFormatChatCompletion},
		{"response canonical", "response", OpenAICompatibleAPIFormatResponse},
		{"response alias", "Response_API", OpenAICompatibleAPIFormatResponse},
		{"response plural", "responses", OpenAICompatibleAPIFormatResponse},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeOpenAICompatibleAPIFormat(tc.input)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestUseOpenAICompatibleResponseAPI(t *testing.T) {
	t.Parallel()

	require.True(t, UseOpenAICompatibleResponseAPI("response"), "expected response format to enable Response API")
	require.False(t, UseOpenAICompatibleResponseAPI("chat"), "expected chat format to disable Response API")
}
