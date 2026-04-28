package palm

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/render"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/constant"
	"github.com/Laisky/one-api/relay/model"
)

// https://developers.generativeai.google/api/rest/generativelanguage/models/generateMessage#request-body
// https://developers.generativeai.google/api/rest/generativelanguage/models/generateMessage#response-body

func ConvertRequest(textRequest model.GeneralOpenAIRequest) *ChatRequest {
	palmRequest := ChatRequest{
		Prompt: Prompt{
			Messages: make([]ChatMessage, 0, len(textRequest.Messages)),
		},
		Temperature: textRequest.Temperature,
		TopP:        textRequest.TopP,
		TopK:        textRequest.MaxTokens,
	}

	if textRequest.N != nil {
		palmRequest.CandidateCount = *textRequest.N
	} else {
		palmRequest.CandidateCount = 1
	}

	for _, message := range textRequest.Messages {
		palmMessage := ChatMessage{
			Content: message.StringContent(),
		}
		if message.Role == "user" {
			palmMessage.Author = "0"
		} else {
			palmMessage.Author = "1"
		}
		palmRequest.Prompt.Messages = append(palmRequest.Prompt.Messages, palmMessage)
	}
	return &palmRequest
}

func responsePaLM2OpenAI(response *ChatResponse) *openai.TextResponse {
	fullTextResponse := openai.TextResponse{
		Choices: make([]openai.TextResponseChoice, 0, len(response.Candidates)),
	}
	for i, candidate := range response.Candidates {
		choice := openai.TextResponseChoice{
			Index: i,
			Message: model.Message{
				Role:    "assistant",
				Content: candidate.Content,
			},
			FinishReason: "stop",
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func streamResponsePaLM2OpenAI(palmResponse *ChatResponse) *openai.ChatCompletionsStreamResponse {
	var choice openai.ChatCompletionsStreamResponseChoice
	if len(palmResponse.Candidates) > 0 {
		choice.Delta.Content = palmResponse.Candidates[0].Content
	}
	choice.FinishReason = &constant.StopFinishReason
	var response openai.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Model = "palm2"
	response.Choices = []openai.ChatCompletionsStreamResponseChoice{choice}
	return &response
}

func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, string) {
	lg := gmw.GetLogger(c)
	responseText := ""
	responseId := tracing.GenerateChatCompletionID(c)
	createdTime := helper.GetTimestamp()

	common.SetEventStreamHeaders(c)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		lg.Error("error reading stream response", zap.Error(err))
		err := resp.Body.Close()
		if err != nil {
			return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), ""
		}
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), ""
	}

	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), ""
	}

	var palmResponse ChatResponse
	err = json.Unmarshal(responseBody, &palmResponse)
	if err != nil {
		lg.Error("error unmarshalling stream response", zap.Error(err))
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), ""
	}

	fullTextResponse := streamResponsePaLM2OpenAI(&palmResponse)
	fullTextResponse.Id = responseId
	fullTextResponse.Created = createdTime
	if len(palmResponse.Candidates) > 0 {
		responseText = palmResponse.Candidates[0].Content
	}

	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		lg.Error("error marshalling stream response", zap.Error(err))
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), ""
	}

	err = render.ObjectData(c, string(jsonResponse))
	if err != nil {
		lg.Error("error rendering response", zap.Error(err))
	}

	render.Done(c)

	return nil, responseText
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
	var palmResponse ChatResponse
	err = json.Unmarshal(responseBody, &palmResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if palmResponse.Error.Code != 0 || len(palmResponse.Candidates) == 0 {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  palmResponse.Error.Message,
				Type:     model.ErrorType(palmResponse.Error.Status),
				Param:    "",
				Code:     palmResponse.Error.Code,
				RawError: errors.New(palmResponse.Error.Message),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := responsePaLM2OpenAI(&palmResponse)
	fullTextResponse.Model = modelName
	completionTokens := openai.CountTokenText(palmResponse.Candidates[0].Content, modelName)
	usage := model.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
	fullTextResponse.Usage = usage
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &usage
}
