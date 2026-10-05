package controller

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestClaude431NativeImageReservation checks URL, file and nested images at both estimation sizes.
func TestClaude431NativeImageReservation(t *testing.T) {
	for _, wire := range []string{"claude-sonnet-5-5", "my-deployment"} {
		for _, blocks := range []string{
			`[{"type":"image","source":{"type":"url","url":"https://image.invalid/test.png","detail":"low"}}]`,
			`[{"type":"image","source":{"type":"file","file_id":"test-image"}}]`,
			`[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"image","source":{"type":"file","file_id":"test-image"}}]}]`,
		} {
			var request, plain ClaudeMessagesRequest
			require.NoError(t, json.Unmarshal([]byte(`{"model":"`+wire+`","messages":[{"role":"user","content":`+blocks+`}],"max_tokens":64}`), &request))
			require.NoError(t, json.Unmarshal([]byte(`{"model":"`+wire+`","messages":[{"role":"user","content":[]}],"max_tokens":64}`), &plain))
			request.CompatibilityModel = "claude-sonnet-5-5"
			plain.CompatibilityModel = "claude-sonnet-5-5"
			for _, size := range []int{0, fastTokenEstimateThreshold + 100} {
				require.Equal(t, 4784, requireClaudePromptEstimate(t, context.Background(), &request, size)-requireClaudePromptEstimate(t, context.Background(), &plain, size))
			}
		}
	}
}
