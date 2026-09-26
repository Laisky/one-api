package aws

// init registers the published Opus 5.5 Bedrock ID before the parent registry
// reads AwsModelIDMap. Account permissions remain the operator's responsibility.
// Source: https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-5-5.html
func init() {
	AwsModelIDMap["claude-opus-5-5"] = "anthropic.claude-opus-5-5"
}
