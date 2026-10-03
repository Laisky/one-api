package aws

// init registers the published Sonnet 5.5 Bedrock ID before the parent adaptor
// builds its dispatch registry. It takes no arguments and returns no values.
// Profile selection remains separate; the runtime requires a global profile.
// Source (2026-09-28):
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html
func init() {
	AwsModelIDMap["claude-sonnet-5-5"] = "anthropic.claude-sonnet-5-5"
}
