package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeFable52GlobalProfiles checks that Bedrock sends these models to a
// documented profile and does not invent GovCloud or EU geo profiles.
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
			require.Equal(t, id, ConvertModelID2CrossRegionProfile(context.Background(), id, "us-gov-east-1"))
		})
	}
}
