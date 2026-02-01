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

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/constant"
	"github.com/songquanpeng/one-api/relay/model"
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
	scanner := bufio.NewScanner(resp.Body)
	helper.ConfigureScannerBuffer(scanner)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := strings.Index(string(data), "}\n"); i >= 0 {
			return i + 2, data[0 : i+1], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	common.SetEventStreamHeaders(c)

	for scanner.Scan() {
		data := scanner.Text()
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
	}

	if err := scanner.Err(); err != nil {
		lg.Error("error reading stream", zap.Error(err), zap.Int("scanner_max_token_size", helper.DefaultScannerMaxTokenSize))
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
