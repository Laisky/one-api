package aws

// claudeSeptember2026BedrockModelIDs maps the Claude 5.1 API model IDs released
// on 2026-09-01 to their Amazon Bedrock model IDs.
// Source: https://platform.claude.com/docs/en/models/overview
var claudeSeptember2026BedrockModelIDs = map[string]string{
	"claude-fable-5-1":  "anthropic.claude-fable-5-1",
	"claude-mythos-5-1": "anthropic.claude-mythos-5-1",
}

func init() {
	for model, bedrockModelID := range claudeSeptember2026BedrockModelIDs {
		AwsModelIDMap[model] = bedrockModelID
	}
}
