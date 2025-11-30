package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestClaudeStructuredOutputCost_NoSurcharge(t *testing.T) {
	completionTokens := 1000
	modelRatio := 0.25

	req := &ClaudeMessagesRequest{
		Tools:     []relaymodel.ClaudeTool{{Name: "t", Description: "d"}},
		Model:     "claude-3-haiku-20240307",
		MaxTokens: 128,
	}

	costBase := calculateClaudeStructuredOutputCost(req, completionTokens, modelRatio, 1.0)
	costDouble := calculateClaudeStructuredOutputCost(req, completionTokens, modelRatio, 2.0)

	require.Equal(t, int64(0), costBase, "expected no structured output surcharge for base")
	require.Equal(t, int64(0), costDouble, "expected no structured output surcharge for double")
}
