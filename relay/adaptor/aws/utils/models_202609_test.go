package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeSeptember2026GlobalInferenceProfiles verifies that both Claude 5.1
// Bedrock model IDs resolve to registered global inference profiles. It accepts
// a testing handle and returns no value.
func TestClaudeSeptember2026GlobalInferenceProfiles(t *testing.T) {
	t.Parallel()

	sourceRegions := GlobalProfileSourceRegions[claudeFable5BedrockModelID]
	require.NotEmpty(t, sourceRegions)

	ctx := context.Background()
	for _, model := range []string{claudeFable51BedrockModelID, claudeMythos51BedrockModelID} {
		profile := "global." + model
		require.ElementsMatch(t, sourceRegions, GlobalProfileSourceRegions[model])
		require.Contains(t, CrossRegionInferences, profile)
		require.Equal(t, profile, ConvertModelID2CrossRegionProfile(ctx, model, "us-east-1"))
		require.Equal(t, profile, ConvertModelID2CrossRegionProfile(ctx, model, "eu-west-1"))
	}
}
