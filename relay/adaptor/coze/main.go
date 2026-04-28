package coze

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/conv"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/coze/constant/messagetype"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// https://www.coze.com/open

func stopReasonCoze2OpenAI(reason *string) string {
	if reason == nil {
		return ""
	}
	switch *reason {
	case "end_turn":
		return "stop"
	case "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	default:
		return *reason
	}
}

func ConvertRequest(textRequest model.GeneralOpenAIRequest) *Request {
	cozeRequest := Request{
		Stream: textRequest.Stream,
		User:   textRequest.User,
		BotId:  strings.TrimPrefix(textRequest.Model, "bot-"),
	}
	for i, message := range textRequest.Messages {
		if i == len(textRequest.Messages)-1 {
			cozeRequest.Query = message.StringContent()
			continue
		}
		cozeMessage := Message{
			Role:    message.Role,
			Content: message.StringContent(),
		}
		cozeRequest.ChatHistory = append(cozeRequest.ChatHistory, cozeMessage)
	}
	return &cozeRequest
}

func StreamResponseCoze2OpenAI(cozeResponse *StreamResponse) (*openai.ChatCompletionsStreamResponse, *Response) {
	var response *Response
	var stopReason string
	var choice openai.ChatCompletionsStreamResponseChoice

	if cozeResponse.Message != nil {
		if cozeResponse.Message.Type != messagetype.Answer {
			return nil, nil
		}
		choice.Delta.Content = cozeResponse.Message.Content
	}
	choice.Delta.Role = "assistant"
	finishReason := stopReasonCoze2OpenAI(&stopReason)
	if finishReason != "null" {
		choice.FinishReason = &finishReason
	}
	var openaiResponse openai.ChatCompletionsStreamResponse
	openaiResponse.Object = "chat.completion.chunk"
	openaiResponse.Choices = []openai.ChatCompletionsStreamResponseChoice{choice}
	openaiResponse.Id = cozeResponse.ConversationId
	return &openaiResponse, response
}

func ResponseCoze2OpenAI(c *gin.Context, cozeResponse *Response) *openai.TextResponse {
	var responseText string
	for _, message := range cozeResponse.Messages {
		if message.Type == messagetype.Answer {
			responseText = message.Content
			break
		}
	}
	choice := openai.TextResponseChoice{
		Index: 0,
		Message: model.Message{
			Role:    "assistant",
			Content: responseText,
			Name:    nil,
		},
		FinishReason: "stop",
	}
	fullTextResponse := openai.TextResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Model:   "coze-bot",
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: []openai.TextResponseChoice{choice},
	}
	return &fullTextResponse
}

func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *string) {
	lg := gmw.GetLogger(c)
	var responseText string
	createdTime := helper.GetTimestamp()
	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	common.SetEventStreamHeaders(c)
	var modelName string

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
			var cozeResponse StreamResponse
			if err := json.NewDecoder(line.Large).Decode(&cozeResponse); err != nil {
				lg.Error("error unmarshalling oversized stream response", zap.Error(err))
				continue
			}

			response, _ := StreamResponseCoze2OpenAI(&cozeResponse)
			if response == nil {
				continue
			}

			for _, choice := range response.Choices {
				responseText += conv.AsString(choice.Delta.Content)
			}
			response.Model = modelName
			response.Created = createdTime

			if err := render.ObjectData(c, response); err != nil {
				lg.Error("error rendering stream response", zap.Error(err))
			}
			continue
		}

		data := line.Text()
		if len(data) < 5 || !strings.HasPrefix(data, "data:") {
			continue
		}
		data = strings.TrimPrefix(data, "data:")
		data = strings.TrimSuffix(data, "\r")

		var cozeResponse StreamResponse
		err = json.Unmarshal([]byte(data), &cozeResponse)
		if err != nil {
			lg.Error("error unmarshalling stream response", zap.Error(err))
			continue
		}

		response, _ := StreamResponseCoze2OpenAI(&cozeResponse)
		if response == nil {
			continue
		}

		for _, choice := range response.Choices {
			responseText += conv.AsString(choice.Delta.Content)
		}
		response.Model = modelName
		response.Created = createdTime

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

	return nil, &responseText
}

func Handler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *string) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	var cozeResponse Response
	err = json.Unmarshal(responseBody, &cozeResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if cozeResponse.Code != 0 {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  cozeResponse.Msg,
				Code:     cozeResponse.Code,
				RawError: errors.New(cozeResponse.Msg),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := ResponseCoze2OpenAI(c, &cozeResponse)
	fullTextResponse.Model = modelName
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	var responseText string
	if len(fullTextResponse.Choices) > 0 {
		responseText = fullTextResponse.Choices[0].Message.StringContent()
	}
	return nil, &responseText
}
