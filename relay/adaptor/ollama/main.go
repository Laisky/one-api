package ollama

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/image"
	"github.com/Laisky/one-api/common/render"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/constant"
	"github.com/Laisky/one-api/relay/model"
)

func ConvertRequest(request model.GeneralOpenAIRequest) *ChatRequest {
	ollamaRequest := ChatRequest{
		Model: request.Model,
		Options: &Options{
			Seed:             int(request.Seed),
			Temperature:      request.Temperature,
			TopP:             request.TopP,
			FrequencyPenalty: request.FrequencyPenalty,
			PresencePenalty:  request.PresencePenalty,
			NumPredict:       request.MaxTokens,
			NumCtx:           request.NumCtx,
		},
		Stream: request.Stream,
	}
	if ollamaRequest.Options.NumPredict == 0 {
		ollamaRequest.Options.NumPredict = config.DefaultMaxToken
	}
	for _, message := range request.Messages {
		openaiContent := message.ParseContent()
		var imageUrls []string
		var contentText string
		for _, part := range openaiContent {
			switch part.Type {
			case model.ContentTypeText:
				if part.Text != nil {
					contentText = *part.Text
				}
			case model.ContentTypeImageURL:
				_, data, _ := image.GetImageFromUrl(part.ImageURL.Url)
				imageUrls = append(imageUrls, data)
			}
		}
		ollamaRequest.Messages = append(ollamaRequest.Messages, Message{
			Role:    message.Role,
			Content: contentText,
			Images:  imageUrls,
		})
	}
	return &ollamaRequest
}

func responseOllama2OpenAI(c *gin.Context, response *ChatResponse) *openai.TextResponse {
	choice := openai.TextResponseChoice{
		Index: 0,
		Message: model.Message{
			Role:    response.Message.Role,
			Content: response.Message.Content,
		},
	}
	if response.Done {
		choice.FinishReason = "stop"
	}
	fullTextResponse := openai.TextResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Model:   response.Model,
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: []openai.TextResponseChoice{choice},
		Usage: model.Usage{
			PromptTokens:     response.PromptEvalCount,
			CompletionTokens: response.EvalCount,
			TotalTokens:      response.PromptEvalCount + response.EvalCount,
		},
	}
	return &fullTextResponse
}

func streamResponseOllama2OpenAI(c *gin.Context, ollamaResponse *ChatResponse) *openai.ChatCompletionsStreamResponse {
	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Role = ollamaResponse.Message.Role
	choice.Delta.Content = ollamaResponse.Message.Content
	if ollamaResponse.Done {
		choice.FinishReason = &constant.StopFinishReason
	}
	response := openai.ChatCompletionsStreamResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion.chunk",
		Created: helper.GetTimestamp(),
		Model:   ollamaResponse.Model,
		Choices: []openai.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
	var usage model.Usage
	reader := bufio.NewReaderSize(resp.Body, 64*1024)

	common.SetEventStreamHeaders(c)

	var streamErr error
	for {
		data, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				if data == "" {
					break
				}
			} else {
				streamErr = readErr
				break
			}
		}

		data = strings.TrimSuffix(data, "\n")
		data = strings.TrimSuffix(data, "\r")
		if after, ok := strings.CutPrefix(data, "}"); ok {
			data = after + "}"
		}

		var ollamaResponse ChatResponse
		err := json.Unmarshal([]byte(data), &ollamaResponse)
		if err != nil {
			lg.Error("error unmarshalling stream response", zap.Error(err))
			continue
		}

		if ollamaResponse.EvalCount != 0 {
			usage.PromptTokens = ollamaResponse.PromptEvalCount
			usage.CompletionTokens = ollamaResponse.EvalCount
			usage.TotalTokens = ollamaResponse.PromptEvalCount + ollamaResponse.EvalCount
		}

		response := streamResponseOllama2OpenAI(c, &ollamaResponse)
		err = render.ObjectData(c, response)
		if err != nil {
			lg.Error("error rendering response", zap.Error(err))
		}

		if readErr == io.EOF {
			break
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

	return nil, &usage
}

func ConvertEmbeddingRequest(request model.GeneralOpenAIRequest) *EmbeddingRequest {
	return &EmbeddingRequest{
		Model: request.Model,
		Input: request.ParseInput(),
		Options: &Options{
			Seed:             int(request.Seed),
			Temperature:      request.Temperature,
			TopP:             request.TopP,
			FrequencyPenalty: request.FrequencyPenalty,
			PresencePenalty:  request.PresencePenalty,
		},
	}
}

func EmbeddingHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var ollamaResponse EmbeddingResponse
	err := json.NewDecoder(resp.Body).Decode(&ollamaResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	if ollamaResponse.Error != "" {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  ollamaResponse.Error,
				Type:     model.ErrorTypeOllama,
				Param:    "",
				Code:     "ollama_error",
				RawError: errors.New(ollamaResponse.Error),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}

	fullTextResponse := embeddingResponseOllama2OpenAI(&ollamaResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

func embeddingResponseOllama2OpenAI(response *EmbeddingResponse) *openai.EmbeddingResponse {
	openAIEmbeddingResponse := openai.EmbeddingResponse{
		Object: "list",
		Data:   make([]openai.EmbeddingResponseItem, 0, 1),
		Model:  response.Model,
		Usage:  model.Usage{TotalTokens: 0},
	}

	for i, embedding := range response.Embeddings {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, openai.EmbeddingResponseItem{
			Object:    `embedding`,
			Index:     i,
			Embedding: embedding,
		})
	}
	return &openAIEmbeddingResponse
}

func Handler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var ollamaResponse ChatResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	gmw.GetLogger(c).Debug("ollama response", zap.ByteString("body", responseBody))
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &ollamaResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if ollamaResponse.Error != "" {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  ollamaResponse.Error,
				Type:     model.ErrorTypeOllama,
				Param:    "",
				Code:     "ollama_error",
				RawError: errors.New(ollamaResponse.Error),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := responseOllama2OpenAI(c, &ollamaResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}
