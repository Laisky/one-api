package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeFable52GlobalProfiles checks deterministic candidate selection without
// inventing GovCloud or EU profiles. The invocation resolver separately rejects
// raw 5.2 candidates before network access; legacy raw-model routing stays intact.
func TestClaudeFable52GlobalProfiles(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"anthropic.claude-fable-5-2", "anthropic.claude-mythos-5-2"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			require.ElementsMatch(t, claudeFable52GlobalRegions, GlobalProfileSourceRegions[id])
			require.Contains(t, CrossRegionInferences, "global."+id)
			require.Contains(t, CrossRegionInferences, "us."+id)
			require.NotContains(t, CrossRegionInferences, "eu."+id)
			require.Equal(t, "global."+id, ConvertModelID2CrossRegionProfile(context.Background(), id, "ca-central-1"))
			require.Equal(t, "global."+id, ConvertModelID2CrossRegionProfile(context.Background(), id, "eu-west-1"))
			require.Equal(t, "global."+id, ConvertModelID2CrossRegionProfile(context.Background(), "global."+id, "us-east-1"))
			require.Equal(t, "us."+id, ConvertModelID2CrossRegionProfile(context.Background(), "us."+id, "us-east-1"))
			require.True(t, RequiresInferenceProfile(id))
			require.Equal(t, id, ConvertModelID2CrossRegionProfile(context.Background(), id, "us-gov-east-1"))
		})
	}
}
