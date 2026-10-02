// Package aws provides the AWS Claude adaptor for the relay service.
package aws

import (
	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// AwsModelIDMap maps public aliases to AWS model IDs. Profile selection is
// separate from this catalog and does not probe inference endpoints.
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html
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
}

// AwsModelID resolves a known public model alias to its AWS ID or returns an
// error. Account access remains an administrator/upstream decision.
func AwsModelID(requestModel string) (string, error) {
	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
		return awsModelID, nil
	}
	return "", errors.Errorf("model %s not found", requestModel)
}

// AwsClaudeModelTransArn returns the channel's explicit inference profile ARN
// for the requested model, or an empty string when no override is configured.
// The client parameter is retained for compatibility and is never invoked.
func AwsClaudeModelTransArn(c *gin.Context, _ *bedrockruntime.Client) string {
	reqModelID := c.GetString(ctxkey.RequestModel)
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok && channel != nil {
			arnMap := channel.GetInferenceProfileArnMapWithContext(gmw.Ctx(c))
			if arnMap != nil {
				return arnMap[reqModelID]
			}
		}
	}
	return ""
}
