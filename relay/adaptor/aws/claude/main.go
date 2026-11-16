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
