package aws

import (
	"github.com/Laisky/errors/v2"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

var _ utils.AwsAdapter = new(Adaptor)

type Adaptor struct {
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	claudeReq, err := anthropic.ConvertRequest(c, *request)
	if err != nil {
		return nil, errors.Wrap(err, "convert request")
	}
	claudeReq.AnthropicVersion = "bedrock-2023-05-31"
	claudeReq.MaxTokens = request.MaxTokens
	if claudeReq.MaxTokens == 0 {
		claudeReq.MaxTokens = config.DefaultMaxToken
	}
	claudeReq.TopK = request.TopK
	claudeReq.TopP = request.TopP
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, claudeReq)
	return claudeReq, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// AWS Bedrock supports Claude Messages natively. Do not convert payload.
	// Just set context for billing/routing and mark direct pass-through.
	c.Set(ctxkey.ClaudeMessagesNative, true)
	c.Set(ctxkey.ClaudeDirectPassthrough, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)
	c.Set(ctxkey.RequestModel, request.Model)
	// Also parse into anthropic.Request for AWS SDK payload building
	if parsed, perr := anthropic.ConvertClaudeRequest(c, *request); perr == nil {
		c.Set(ctxkey.ConvertedRequest, parsed)
	} else {
		return nil, perr
	}
	// Return the original request object; controller will forward original body
	return request, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, awsCli *bedrockruntime.Client, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = StreamHandler(c, awsCli)
	} else {
		err, usage = Handler(c, awsCli, meta.ActualModelName)
	}
	return
}
