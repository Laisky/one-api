package vertexai

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
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

// ConvertRequest converts an OpenAI-compatible request into the Vertex AI Gemini payload expected by the selected relay mode.
// Parameters: c is the request context, relayMode selects the endpoint family, and request is the validated OpenAI-compatible request.
// Returns: the converted Vertex AI Gemini payload or an error when conversion fails.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	var (
		convertedRequest any
		geminiRequest    *gemini.ChatRequest
	)
	if relayMode == relaymode.Embeddings {
		embeddingRequest, err := gemini.ConvertEmbeddingRequest(*request)
		if err != nil {
			return nil, errors.Wrap(err, "convert vertex AI gemini embedding request")
		}
		convertedRequest = embeddingRequest
	} else {
		geminiRequest = gemini.ConvertRequest(*request)
		convertedRequest = geminiRequest
	}

	lg := gmw.GetLogger(c)
	if convertedRequest == nil {
		lg.Error("gemini request conversion returned nil",
			zap.String("model", request.Model))
		return nil, errors.New("converted request is nil")
	}

	lastRole := ""
	if geminiRequest != nil && len(geminiRequest.Contents) > 0 {
		lastRole = geminiRequest.Contents[len(geminiRequest.Contents)-1].Role
	}

	lg.Debug("gemini vertex convert summary",
		zap.String("model", request.Model),
		zap.Int("content_count", func() int {
			if geminiRequest == nil {
				return 0
			}
			return len(geminiRequest.Contents)
		}()),
		zap.String("last_role", lastRole),
		zap.Bool("has_system_instruction", geminiRequest != nil && geminiRequest.SystemInstruction != nil),
	)

	var functionNames []string
	var parameterTypes []string
	if geminiRequest != nil {
		for _, tool := range geminiRequest.Tools {
			functions, ok := tool.FunctionDeclarations.([]model.Function)
			if !ok {
				continue
			}

			for _, fn := range functions {
				functionNames = append(functionNames, fn.Name)
				if params, ok := fn.Parameters.(map[string]any); ok {
					if typeVal, ok := params["type"].(string); ok {
						parameterTypes = append(parameterTypes, typeVal)
						continue
					}
				}
				parameterTypes = append(parameterTypes, "")
			}
		}
	}

	if len(functionNames) > 0 {
		lg.Debug("gemini vertex tools normalized",
			zap.Int("function_count", len(functionNames)),
			zap.Strings("function_names", functionNames),
			zap.Strings("function_param_types", parameterTypes),
		)
	}
	c.Set(ctxkey.RequestModel, request.Model)
	c.Set(ctxkey.ConvertedRequest, convertedRequest)
	return convertedRequest, nil
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
			err, usage = gemini.EmbeddingHandler(c, resp, meta.PromptTokens)
		default:
			err, usage = gemini.Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}
	return
}
