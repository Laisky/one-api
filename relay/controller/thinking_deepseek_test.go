package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestApplyThinkingQueryToDeepSeekV4NormalizesReasoningEffort verifies the
// query-parameter path applies the same DeepSeek V4 reasoning mapping as JSON bodies.
// Parameters: t is the testing handle used for assertions and subtests.
// Returns: nothing; the test fails when query normalization diverges from the provider mapping.
func TestApplyThinkingQueryToDeepSeekV4NormalizesReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "low remains low", input: "low", expected: "low"},
		{name: "medium maps to high", input: "medium", expected: "high"},
		{name: "high remains high", input: "high", expected: "high"},
		{name: "xhigh maps to high", input: "xhigh", expected: "high"},
		{name: "max remains max", input: "max", expected: "max"},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			writer := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest(
				http.MethodPost,
				"/v1/chat/completions?thinking=true&reasoning_effort="+testCase.input,
				nil,
			)

			meta := &metalib.Meta{
				ActualModelName: "deepseek-v4-pro",
				APIType:         apitype.OpenAI,
				ChannelType:     channeltype.DeepSeek,
			}
			payload := &relaymodel.GeneralOpenAIRequest{Model: meta.ActualModelName}

			applyThinkingQueryToChatRequest(c, payload, meta)

			require.NotNil(t, payload.ReasoningEffort)
			require.Equal(t, testCase.expected, *payload.ReasoningEffort)
		})
	}
}
