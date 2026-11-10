package aws

// init registers official Bedrock IDs before the parent dispatch registry loads.
// Access eligibility and inference-profile selection remain external concerns.
// Source: https://platform.claude.com/docs/en/models/overview
func init() {
	AwsModelIDMap["claude-fable-5-2"] = "anthropic.claude-fable-5-2"
	AwsModelIDMap["claude-mythos-5-2"] = "anthropic.claude-mythos-5-2"
}
