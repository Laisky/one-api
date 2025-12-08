package vertexai

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor/gemini"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// ModelList is the list of models supported by Vertex AI.
//
// https://cloud.google.com/vertex-ai/generative-ai/docs/learn/models
// var ModelList = []string{
// 	"gemini-pro", "gemini-pro-vision",
// 	"gemini-exp-1206",
// 	"gemini-1.0-pro",
// 	"gemini-1.0-pro-vision",
// 	"gemini-1.5-pro", "gemini-1.5-pro-001", "gemini-1.5-pro-002",
// 	"gemini-1.5-flash", "gemini-1.5-flash-001", "gemini-1.5-flash-002",
// 	"gemini-2.0-flash", "gemini-2.0-flash-exp", "gemini-2.0-flash-001",
// 	"gemini-2.0-flash-lite", "gemini-2.0-flash-lite-001",
// 	"gemini-2.0-flash-thinking-exp-01-21",
// 	"gemini-2.0-pro-exp-02-05",
// }

type Adaptor struct {
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	geminiRequest := gemini.ConvertRequest(*request)
	lg := gmw.GetLogger(c)
	if geminiRequest == nil {
		lg.Error("gemini request conversion returned nil",
			zap.String("model", request.Model))
		return nil, errors.New("converted request is nil")
	}

	lastRole := ""
	if len(geminiRequest.Contents) > 0 {
		lastRole = geminiRequest.Contents[len(geminiRequest.Contents)-1].Role
	}

	lg.Debug("gemini vertex convert summary",
		zap.String("model", request.Model),
		zap.Int("content_count", len(geminiRequest.Contents)),
		zap.String("last_role", lastRole),
		zap.Bool("has_system_instruction", geminiRequest.SystemInstruction != nil),
	)
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, geminiRequest)
	return geminiRequest, nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("not support image request")
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		var responseText string
		err, responseText = gemini.StreamHandler(c, resp)
		usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
	} else {
		switch meta.Mode {
		case relaymode.Embeddings:
			err, usage = gemini.EmbeddingHandler(c, resp)
		default:
			err, usage = gemini.Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}
	return
}
