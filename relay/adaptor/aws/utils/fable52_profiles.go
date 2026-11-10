package utils

import "slices"

// claudeFable52GlobalRegions contains AWS-documented commercial source
// regions for both Claude 5.2 global profiles; GovCloud has no global profile.
// Sources:
//   - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-fable-5-2.html
//   - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-mythos-5-2.html
var claudeFable52GlobalRegions = []string{
	"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1", "ca-west-1",
	"eu-central-1", "eu-central-2", "eu-north-1", "eu-south-1", "eu-south-2", "eu-west-1", "eu-west-2", "eu-west-3",
	"ap-east-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-south-1", "ap-south-2",
	"ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-4", "ap-southeast-5", "ap-southeast-6", "ap-southeast-7",
	"il-central-1", "me-central-1", "me-south-1", "af-south-1", "sa-east-1", "mx-central-1",
}

// init registers the published global and US geo inference IDs. The normal
// resolver selects global for supported regions, never an in-region model ID.
func init() {
	for _, id := range []string{"anthropic.claude-fable-5-2", "anthropic.claude-mythos-5-2"} {
		GlobalProfileSourceRegions[id] = slices.Clone(claudeFable52GlobalRegions)
		CrossRegionInferences = append(CrossRegionInferences, "global."+id, "us."+id)
	}
}

// RequiresInferenceProfile reports whether a verified model catalog forbids raw
// on-demand invocation. Older models keep their historical routing behavior.
func RequiresInferenceProfile(modelID string) bool {
	switch modelID {
	case "anthropic.claude-fable-5-2", "anthropic.claude-mythos-5-2":
		return true
	default:
		return false
	}
}
