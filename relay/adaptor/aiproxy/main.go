package aiproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/constant"
	"github.com/Laisky/one-api/relay/model"
)

// https://docs.aiproxy.io/dev/library#使用已经定制好的知识库进行对话问答

func ConvertRequest(request model.GeneralOpenAIRequest) *LibraryRequest {
	query := ""
	if len(request.Messages) != 0 {
		query = request.Messages[len(request.Messages)-1].StringContent()
	}
	return &LibraryRequest{
		Model:  request.Model,
		Stream: request.Stream,
		Query:  query,
	}
}

func aiProxyDocuments2Markdown(documents []LibraryDocument) string {
	if len(documents) == 0 {
		return ""
	}
	content := "\n\nReference Documents:\n"
	for i, document := range documents {
		content += fmt.Sprintf("%d. [%s](%s)\n", i+1, document.Title, document.URL)
	}
	return content
}

func responseAIProxyLibrary2OpenAI(c *gin.Context, response *LibraryResponse) *openai.TextResponse {
	content := response.Answer + aiProxyDocuments2Markdown(response.Documents)
	choice := openai.TextResponseChoice{
		Index: 0,
		Message: model.Message{
			Role:    "assistant",
			Content: content,
		},
		FinishReason: "stop",
	}
	fullTextResponse := openai.TextResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: []openai.TextResponseChoice{choice},
	}
	return &fullTextResponse
}

func documentsAIProxyLibrary(c *gin.Context, documents []LibraryDocument) *openai.ChatCompletionsStreamResponse {
	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Content = aiProxyDocuments2Markdown(documents)
	choice.FinishReason = &constant.StopFinishReason
	return &openai.ChatCompletionsStreamResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion.chunk",
		Created: helper.GetTimestamp(),
		Model:   "",
		Choices: []openai.ChatCompletionsStreamResponseChoice{choice},
	}
}

func streamResponseAIProxyLibrary2OpenAI(c *gin.Context, response *LibraryStreamResponse) *openai.ChatCompletionsStreamResponse {
	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Content = response.Content
	return &openai.ChatCompletionsStreamResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion.chunk",
		Created: helper.GetTimestamp(),
		Model:   response.Model,
		Choices: []openai.ChatCompletionsStreamResponseChoice{choice},
	}
}

func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var usage model.Usage
	var documents []LibraryDocument
	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	common.SetEventStreamHeaders(c)

	lg := gmw.GetLogger(c)
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
			var aiProxyLibraryResponse LibraryStreamResponse
			if err := json.NewDecoder(line.Large).Decode(&aiProxyLibraryResponse); err != nil {
				lg.Error("error unmarshalling oversized stream response", zap.Error(err))
				continue
			}
			if len(aiProxyLibraryResponse.Documents) != 0 {
				documents = aiProxyLibraryResponse.Documents
			}
			response := streamResponseAIProxyLibrary2OpenAI(c, &aiProxyLibraryResponse)
			if err := render.ObjectData(c, response); err != nil {
				lg.Error("render object data error", zap.Error(err))
			}
			continue
		}

		data := line.Text()
		if len(data) < 5 || data[:5] != "data:" {
			continue
		}
		data = data[5:]

		var AIProxyLibraryResponse LibraryStreamResponse
		err = json.Unmarshal([]byte(data), &AIProxyLibraryResponse)
		if err != nil {
			lg.Error("error unmarshalling stream response", zap.Error(err))
			continue
		}
		if len(AIProxyLibraryResponse.Documents) != 0 {
			documents = AIProxyLibraryResponse.Documents
		}
		response := streamResponseAIProxyLibrary2OpenAI(c, &AIProxyLibraryResponse)
		err = render.ObjectData(c, response)
		if err != nil {
			lg.Error("render object data error", zap.Error(err))
		}
	}

	if streamErr != nil {
		lg.Error("error reading stream", zap.Error(streamErr))
	}

	response := documentsAIProxyLibrary(c, documents)
	err := render.ObjectData(c, response)
	if err != nil {
		lg.Error("render object data error", zap.Error(err))
	}
	render.Done(c)

	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	return nil, &usage
}

func Handler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var AIProxyLibraryResponse LibraryResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &AIProxyLibraryResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if AIProxyLibraryResponse.ErrCode != 0 {
		errType := model.ErrorType(strconv.Itoa(AIProxyLibraryResponse.ErrCode))
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  AIProxyLibraryResponse.Message,
				Type:     errType,
				Code:     AIProxyLibraryResponse.ErrCode,
				RawError: errors.New(AIProxyLibraryResponse.Message),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := responseAIProxyLibrary2OpenAI(c, &AIProxyLibraryResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), nil
	}
	return nil, &fullTextResponse.Usage
}
