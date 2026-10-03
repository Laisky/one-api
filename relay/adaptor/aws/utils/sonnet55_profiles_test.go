package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClaudeSonnet55GlobalProfiles compares production routing against the AWS
// model card's complete commercial source-region list. It takes a test handle
// and returns nothing; unknown/GovCloud regions must not cross to global.
func TestClaudeSonnet55GlobalProfiles(t *testing.T) {
	t.Parallel()
	const id = "anthropic.claude-sonnet-5-5"
	const global = "global." + id
	regions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1", "ca-west-1",
		"eu-central-1", "eu-central-2", "eu-north-1", "eu-south-1", "eu-south-2", "eu-west-1", "eu-west-2", "eu-west-3",
		"ap-east-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-south-1", "ap-south-2",
		"ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-4", "ap-southeast-5", "ap-southeast-6", "ap-southeast-7",
		"il-central-1", "me-central-1", "me-south-1", "af-south-1", "sa-east-1", "mx-central-1",
	}
	require.ElementsMatch(t, regions, GlobalProfileSourceRegions[id])
	require.Contains(t, CrossRegionInferences, global)
	for _, region := range regions {
		t.Run(region, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, global, ConvertModelID2CrossRegionProfile(context.Background(), id, region))
			require.Equal(t, global, ConvertModelID2CrossRegionProfile(context.Background(), global, region), "already selected profiles remain unchanged")
		})
	}
	for _, region := range []string{"us-gov-east-1", "us-gov-west-1", "cn-north-1", "unknown-region"} {
		t.Run(region, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, id, ConvertModelID2CrossRegionProfile(context.Background(), id, region))
		})
	}
	for _, prefix := range []string{"us", "eu", "au", "jp", "apac", "us-gov"} {
		require.NotContains(t, CrossRegionInferences, prefix+"."+id, "do not manufacture unpublished geographic IDs")
	}
}
