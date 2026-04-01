package cloudflare

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/render"
	commonsse "github.com/songquanpeng/one-api/common/sse"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/model"
)

func ConvertCompletionsRequest(textRequest model.GeneralOpenAIRequest) *Request {
	request := &Request{
		Prompt:      textRequest.Prompt,
		MaxTokens:   textRequest.MaxTokens,
		Stream:      textRequest.Stream,
		Temperature: textRequest.Temperature,
	}
	if request.MaxTokens == 0 {
		request.MaxTokens = config.DefaultMaxToken
	}
	return request
}

func StreamHandler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	common.SetEventStreamHeaders(c)
	id := helper.GetResponseID(c)
	responseModel := c.GetString(ctxkey.RequestModel)
	var responseText string
	var streamErr error

	for {
		line, err := lineReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			streamErr = err
			break
		}

		if line.Oversized {
			var response openai.ChatCompletionsStreamResponse
			if err := json.NewDecoder(line.Large).Decode(&response); err != nil {
				lg.Error("error unmarshalling oversized stream response", zap.Error(err))
				continue
			}
			for _, v := range response.Choices {
				v.Delta.Role = "assistant"
				responseText += v.Delta.StringContent()
			}
			response.Id = id
			response.Model = modelName
			if err := render.ObjectData(c, response); err != nil {
				lg.Error("error rendering stream response", zap.Error(err))
			}
			continue
		}

		data := line.Text()
		if len(data) < len("data: ") {
			continue
		}
		data = strings.TrimPrefix(data, "data: ")
		data = strings.TrimSuffix(data, "\r")

		if data == "[DONE]" {
			break
		}

		var response openai.ChatCompletionsStreamResponse
		err = json.Unmarshal([]byte(data), &response)
		if err != nil {
			lg.Error("error unmarshalling stream response", zap.Error(err))
			continue
		}
		for _, v := range response.Choices {
			v.Delta.Role = "assistant"
			responseText += v.Delta.StringContent()
		}
		response.Id = id
		response.Model = modelName
		err = render.ObjectData(c, response)
		if err != nil {
			lg.Error("error rendering stream response", zap.Error(err))
		}
	}

	if streamErr != nil {
		lg.Error("error reading stream", zap.Error(streamErr))
	}

	render.Done(c)

	err := resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	usage := openai.ResponseText2Usage(responseText, responseModel, promptTokens)
	return nil, usage
}

func Handler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	var response openai.TextResponse
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	response.Model = modelName
	var responseText string
	for _, v := range response.Choices {
		responseText += v.Message.StringContent()
	}
	usage := openai.ResponseText2Usage(responseText, modelName, promptTokens)
	response.Usage = *usage
	response.Id = helper.GetResponseID(c)
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, usage
}
