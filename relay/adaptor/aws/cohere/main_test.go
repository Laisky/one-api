package aws

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Test that convertConverseResponseToCohere maps usage to relaymodel.Usage
// with JSON fields: prompt_tokens, completion_tokens, total_tokens.
func TestConvertConverseResponseToCohereUsageMapping(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	in := int32(11)
	out := int32(22)
	total := int32(33)

	converse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{Value: types.Message{Role: types.ConversationRole("assistant")}},
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(in),
			OutputTokens: aws.Int32(out),
			TotalTokens:  aws.Int32(total),
		},
	}

	resp := convertConverseResponseToCohere(c, converse, "cohere.command-r-v1:0")

	// Marshal to JSON to assert field names
	b, err := json.Marshal(resp)
	require.NoError(t, err, "marshal cohere response")

	// Quick JSON string contains checks for usage keys and values
	js := string(b)
	require.True(t, strings.Contains(js, "\"prompt_tokens\":11"), "expected prompt_tokens:11 in json, got %s", js)
	require.True(t, strings.Contains(js, "\"completion_tokens\":22"), "expected completion_tokens:22 in json, got %s", js)
	require.True(t, strings.Contains(js, "\"total_tokens\":33"), "expected total_tokens:33 in json, got %s", js)
}
