package utils

// init registers only source regions marked Global-supported on the Opus 5.5
// model card. GovCloud is not included; preselected geographic/global profile
// IDs continue to pass through unchanged.
// Source: https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-5-5.html
func init() {
	GlobalProfileSourceRegions["anthropic.claude-opus-5-5"] = []string{
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
	for _, prefix := range []string{"global", "us", "eu", "au", "jp"} {
		CrossRegionInferences = append(CrossRegionInferences, prefix+".anthropic.claude-opus-5-5")
	}
}
