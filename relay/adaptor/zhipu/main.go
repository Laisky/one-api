package zhipu

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/common/render"
	commonsse "github.com/songquanpeng/one-api/common/sse"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/constant"
	"github.com/songquanpeng/one-api/relay/model"
)

// https://open.bigmodel.cn/doc/api#chatglm_std
// chatglm_std, chatglm_lite
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/invoke
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/sse-invoke

var zhipuTokens sync.Map
var expSeconds int64 = 24 * 3600

func GetToken(apikey string) string {
	data, ok := zhipuTokens.Load(apikey)
	if ok {
		tokenData := data.(tokenData)
		if time.Now().Before(tokenData.ExpiryTime) {
			return tokenData.Token
		}
	}

	split := strings.Split(apikey, ".")
	if len(split) != 2 {
		// invalid zhipu key
		return ""
	}

	id := split[0]
	secret := split[1]

	expMillis := time.Now().Add(time.Duration(expSeconds)*time.Second).UnixNano() / 1e6
	expiryTime := time.Now().Add(time.Duration(expSeconds) * time.Second)

	timestamp := time.Now().UnixNano() / 1e6

	payload := jwt.MapClaims{
		"api_key":   id,
		"exp":       expMillis,
		"timestamp": timestamp,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)

	token.Header["alg"] = "HS256"
	token.Header["sign_type"] = "SIGN"

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return ""
	}

	zhipuTokens.Store(apikey, tokenData{
		Token:      tokenString,
		ExpiryTime: expiryTime,
	})

	return tokenString
}

func ConvertRequest(request model.GeneralOpenAIRequest) *Request {
	messages := make([]Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		messages = append(messages, Message{
			Role:    message.Role,
			Content: message.StringContent(),
		})
	}
	return &Request{
		Prompt:      messages,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Incremental: false,
	}
}

func responseZhipu2OpenAI(response *Response) *openai.TextResponse {
	fullTextResponse := openai.TextResponse{
		Id:      response.Data.TaskId,
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: make([]openai.TextResponseChoice, 0, len(response.Data.Choices)),
		Usage:   response.Data.Usage,
	}
	for i, choice := range response.Data.Choices {
		openaiChoice := openai.TextResponseChoice{
			Index: i,
			Message: model.Message{
				Role:    choice.Role,
				Content: strings.Trim(choice.Content, "\""),
			},
			FinishReason: "",
		}
		if i == len(response.Data.Choices)-1 {
			openaiChoice.FinishReason = "stop"
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, openaiChoice)
	}
	return &fullTextResponse
}

func streamResponseZhipu2OpenAI(zhipuResponse string) *openai.ChatCompletionsStreamResponse {
	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Content = zhipuResponse
	response := openai.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: helper.GetTimestamp(),
		Model:   "chatglm",
		Choices: []openai.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

func streamMetaResponseZhipu2OpenAI(zhipuResponse *StreamMetaResponse) (*openai.ChatCompletionsStreamResponse, *model.Usage) {
	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Content = ""
	choice.FinishReason = &constant.StopFinishReason
	response := openai.ChatCompletionsStreamResponse{
		Id:      zhipuResponse.RequestId,
		Object:  "chat.completion.chunk",
		Created: helper.GetTimestamp(),
		Model:   "chatglm",
		Choices: []openai.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response, &zhipuResponse.Usage
}

func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var usage *model.Usage
	lg := gmw.GetLogger(c)
	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	common.SetEventStreamHeaders(c)

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
			payloadBytes, err := io.ReadAll(line.Large)
			if err != nil {
				streamErr = err
				break
			}
			response := streamResponseZhipu2OpenAI(string(payloadBytes))
			if err := render.ObjectData(c, response); err != nil {
				lg.Error("error marshalling oversized stream response", zap.Error(err))
			}
			continue
		}

		lineText := line.Text()
		if len(lineText) < 5 {
			continue
		}
		if strings.HasPrefix(lineText, "data:") {
			response := streamResponseZhipu2OpenAI(lineText[5:])
			if err := render.ObjectData(c, response); err != nil {
				lg.Error("error marshalling stream response", zap.Error(err))
			}
		} else if strings.HasPrefix(lineText, "meta:") {
			metaSegment := lineText[5:]
			var zhipuResponse StreamMetaResponse
			if err := json.Unmarshal([]byte(metaSegment), &zhipuResponse); err != nil {
				lg.Error("error unmarshalling stream response", zap.Error(err))
				continue
			}
			response, zhipuUsage := streamMetaResponseZhipu2OpenAI(&zhipuResponse)
			if err := render.ObjectData(c, response); err != nil {
				lg.Error("error marshalling stream response", zap.Error(err))
			}
			usage = zhipuUsage
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

	return nil, usage
}

func Handler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var zhipuResponse Response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &zhipuResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if !zhipuResponse.Success {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  zhipuResponse.Msg,
				Type:     model.ErrorTypeZhipu,
				Param:    "",
				Code:     zhipuResponse.Code,
				RawError: errors.New(zhipuResponse.Msg),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := responseZhipu2OpenAI(&zhipuResponse)
	fullTextResponse.Model = "chatglm"
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

func EmbeddingsHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var zhipuResponse EmbeddingResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &zhipuResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	fullTextResponse := embeddingResponseZhipu2OpenAI(&zhipuResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

func isOCRModel(modelName string) bool {
	return modelName == "glm-ocr"
}

// ConvertOCRRequest extracts the file URL from OpenAI-style messages
// and converts it to a Zhipu OCR request.
func ConvertOCRRequest(request model.GeneralOpenAIRequest) (*OCRRequest, error) {
	// Look for an image URL in the messages
	for _, msg := range request.Messages {
		if msg.Role != "user" {
			continue
		}
		for _, content := range msg.ParseContent() {
			if content.ImageURL != nil && content.ImageURL.Url != "" {
				return &OCRRequest{
					Model: request.Model,
					File:  content.ImageURL.Url,
				}, nil
			}
		}
	}
	return nil, errors.New("glm-ocr requires an image_url in the message content")
}

// OCRHandler converts the Zhipu layout_parsing response to OpenAI chat completion format.
func OCRHandler(c *gin.Context, resp *http.Response, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	var ocrResponse OCRResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &ocrResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	fullTextResponse := openai.TextResponse{
		Id:      "ocr-" + random.GetUUID(),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []openai.TextResponseChoice{
			{
				Index: 0,
				Message: model.Message{
					Role:    "assistant",
					Content: ocrResponse.Content,
				},
				FinishReason: "stop",
			},
		},
		Usage: ocrResponse.Usage,
	}

	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

func embeddingResponseZhipu2OpenAI(response *EmbeddingResponse) *openai.EmbeddingResponse {
	openAIEmbeddingResponse := openai.EmbeddingResponse{
		Object: "list",
		Data:   make([]openai.EmbeddingResponseItem, 0, len(response.Embeddings)),
		Model:  response.Model,
		Usage: model.Usage{
			PromptTokens:     response.PromptTokens,
			CompletionTokens: response.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
	}

	for _, item := range response.Embeddings {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, openai.EmbeddingResponseItem{
			Object:    `embedding`,
			Index:     item.Index,
			Embedding: item.Embedding,
		})
	}
	return &openAIEmbeddingResponse
}
