package vertexai

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

const anthropicVersion = "vertex-2023-10-16"

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

	req := Request{
		AnthropicVersion: anthropicVersion,
		// Model:            claudeReq.Model,
		Messages:      claudeReq.Messages,
		System:        claudeReq.System,
		MaxTokens:     claudeReq.MaxTokens,
		Temperature:   claudeReq.Temperature,
		TopP:          claudeReq.TopP,
		TopK:          claudeReq.TopK,
		Stream:        claudeReq.Stream,
		Tools:         claudeReq.Tools,
		Thinking:      claudeReq.Thinking,
		OutputConfig:  claudeReq.OutputConfig,
		ToolChoice:    claudeReq.ToolChoice,
		StopSequences: claudeReq.StopSequences,
	}

	if req.Temperature != nil && req.TopP != nil {
		req.TopP = nil
	}

	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, req)
	return req, nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("not support image request")
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		err, usage = anthropic.StreamHandler(c, resp)
	} else {
		err, usage = anthropic.Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	}
	return
}
