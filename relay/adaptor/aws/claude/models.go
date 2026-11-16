package aws

// Catalog sources (existing verified contracts):
// - https://platform.claude.com/docs/en/models/overview
// - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-5-5.html
// - https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-sonnet-5-5.html

// AwsModelIDMap maps public Claude IDs to Bedrock invocation IDs.
var AwsModelIDMap = map[string]string{
	"claude-instant-1.2":         "anthropic.claude-instant-v1",
	"claude-2.0":                 "anthropic.claude-v2",
	"claude-2.1":                 "anthropic.claude-v2:1",
	"claude-3-haiku-20240307":    "anthropic.claude-3-haiku-20240307-v1:0",
	"claude-3-5-haiku-latest":    "anthropic.claude-3-5-haiku-20241022-v1:0",
	"claude-3-5-haiku-20241022":  "anthropic.claude-3-5-haiku-20241022-v1:0",
	"claude-haiku-4-5":           "anthropic.claude-haiku-4-5-20251001-v1:0",
	"claude-haiku-4-5-20251001":  "anthropic.claude-haiku-4-5-20251001-v1:0",
	"claude-3-sonnet-20240229":   "anthropic.claude-3-sonnet-20240229-v1:0",
	"claude-3-5-sonnet-latest":   "anthropic.claude-3-5-sonnet-20241022-v2:0",
	"claude-3-5-sonnet-20240620": "anthropic.claude-3-5-sonnet-20240620-v1:0",
	"claude-3-5-sonnet-20241022": "anthropic.claude-3-5-sonnet-20241022-v2:0",
	"claude-3-7-sonnet-latest":   "anthropic.claude-3-7-sonnet-20250219-v1:0",
	"claude-3-7-sonnet-20250219": "anthropic.claude-3-7-sonnet-20250219-v1:0",
	"claude-sonnet-4-0":          "anthropic.claude-sonnet-4-20250514-v1:0",
	"claude-sonnet-4-20250514":   "anthropic.claude-sonnet-4-20250514-v1:0",
	"claude-sonnet-4-5":          "anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-5-20250929": "anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-sonnet-4-6":          "anthropic.claude-sonnet-4-6",
	"claude-sonnet-5":            "anthropic.claude-sonnet-5",
	"claude-3-opus-20240229":     "anthropic.claude-3-opus-20240229-v1:0",
	"claude-opus-4-0":            "anthropic.claude-opus-4-20250514-v1:0",
	"claude-opus-4-20250514":     "anthropic.claude-opus-4-20250514-v1:0",
	"claude-opus-4-1":            "anthropic.claude-opus-4-1-20250805-v1:0",
	"claude-opus-4-1-20250805":   "anthropic.claude-opus-4-1-20250805-v1:0",
	"claude-opus-4-5":            "anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-5-20251101":   "anthropic.claude-opus-4-5-20251101-v1:0",
	"claude-opus-4-6":            "anthropic.claude-opus-4-6-v1",
	"claude-opus-4-7":            "anthropic.claude-opus-4-7",
	"claude-opus-4-8":            "anthropic.claude-opus-4-8",
	"claude-opus-5":              "anthropic.claude-opus-5",
	"claude-fable-5":             "anthropic.claude-fable-5",
	"claude-fable-5-1":           "anthropic.claude-fable-5-1",
	"claude-mythos-5-1":          "anthropic.claude-mythos-5-1",
	"claude-fable-5-2":           "anthropic.claude-fable-5-2",
	"claude-mythos-5-2":          "anthropic.claude-mythos-5-2",
	"claude-opus-5-5":            "anthropic.claude-opus-5-5",
	"claude-sonnet-5-5":          "anthropic.claude-sonnet-5-5",
}
