package utils

// init registers Sonnet 5.5's documented commercial global source regions.
// It takes no arguments and returns nothing. It neither probes AWS nor invents
// geographic profiles from the older Sonnet 5 region list. GovCloud and unknown
// regions are not silently routed to the commercial global profile.
// Source (2026-09-28):
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html
func init() {
	GlobalProfileSourceRegions["anthropic.claude-sonnet-5-5"] = []string{
		"us-east-1",
		"us-east-2",
		"us-west-1",
		"us-west-2",
		"ca-central-1",
		"ca-west-1",
		"eu-central-1",
		"eu-central-2",
		"eu-north-1",
		"eu-south-1",
		"eu-south-2",
		"eu-west-1",
		"eu-west-2",
		"eu-west-3",
		"ap-east-2",
		"ap-northeast-1",
		"ap-northeast-2",
		"ap-northeast-3",
		"ap-south-1",
		"ap-south-2",
		"ap-southeast-1",
		"ap-southeast-2",
		"ap-southeast-3",
		"ap-southeast-4",
		"ap-southeast-5",
		"ap-southeast-6",
		"ap-southeast-7",
		"il-central-1",
		"me-central-1",
		"me-south-1",
		"af-south-1",
		"sa-east-1",
		"mx-central-1",
	}
	CrossRegionInferences = append(CrossRegionInferences, "global.anthropic.claude-sonnet-5-5")
}
