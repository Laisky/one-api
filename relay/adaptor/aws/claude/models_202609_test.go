package aws

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaudeSeptember2026BedrockModelIDs(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"claude-fable-5-1":  "anthropic.claude-fable-5-1",
		"claude-mythos-5-1": "anthropic.claude-mythos-5-1",
	}

	for model, expected := range tests {
		model := model
		expected := expected
		t.Run(model, func(t *testing.T) {
			t.Parallel()

			actual, err := AwsModelID(model)
			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}
