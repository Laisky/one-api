package baidu

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/constant"
	"github.com/songquanpeng/one-api/relay/model"
)

// https://cloud.baidu.com/doc/WENXINWORKSHOP/s/flfmc9do2

type TokenResponse struct {
	ExpiresIn   int    `json:"expires_in"`
	AccessToken string `json:"access_token"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Messages        []Message `json:"messages"`
	Temperature     *float64  `json:"temperature,omitempty"`
	TopP            *float64  `json:"top_p,omitempty"`
	PenaltyScore    *float64  `json:"penalty_score,omitempty"`
	Stream          bool      `json:"stream,omitempty"`
	System          string    `json:"system,omitempty"`
	DisableSearch   bool      `json:"disable_search,omitempty"`
	EnableCitation  bool      `json:"enable_citation,omitempty"`
	MaxOutputTokens int       `json:"max_output_tokens,omitempty"`
	UserId          string    `json:"user_id,omitempty"`
}

type Error struct {
	ErrorCode int    `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
}

var baiduTokenStore sync.Map

func ConvertRequest(request model.GeneralOpenAIRequest) *ChatRequest {
	baiduRequest := ChatRequest{
		Messages:        make([]Message, 0, len(request.Messages)),
		Temperature:     request.Temperature,
		TopP:            request.TopP,
		PenaltyScore:    request.FrequencyPenalty,
		Stream:          request.Stream,
		DisableSearch:   false,
		EnableCitation:  false,
		MaxOutputTokens: request.MaxTokens,
		UserId:          request.User,
	}
	if baiduRequest.MaxOutputTokens == 0 {
		baiduRequest.MaxOutputTokens = config.DefaultMaxToken
	}
	for _, message := range request.Messages {
		if message.Role == "system" {
			baiduRequest.System = message.StringContent()
		} else {
			baiduRequest.Messages = append(baiduRequest.Messages, Message{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	return &baiduRequest
}

func responseBaidu2OpenAI(response *ChatResponse) *openai.TextResponse {
	choice := openai.TextResponseChoice{
		Index: 0,
		Message: model.Message{
			Role:    "assistant",
			Content: response.Result,
		},
		FinishReason: "stop",
	}
	fullTextResponse := openai.TextResponse{
		Id:      response.Id,
		Object:  "chat.completion",
		Created: response.Created,
		Choices: []openai.TextResponseChoice{choice},
		Usage:   response.Usage,
	}
	return &fullTextResponse
}

func streamResponseBaidu2OpenAI(baiduResponse *ChatStreamResponse) *openai.ChatCompletionsStreamResponse {
	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Content = baiduResponse.Result
	if baiduResponse.IsEnd {
		choice.FinishReason = &constant.StopFinishReason
	}
	response := openai.ChatCompletionsStreamResponse{
		Id:      baiduResponse.Id,
		Object:  "chat.completion.chunk",
		Created: baiduResponse.Created,
		Model:   "ernie-bot",
		Choices: []openai.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

func ConvertEmbeddingRequest(request model.GeneralOpenAIRequest) *EmbeddingRequest {
	return &EmbeddingRequest{
		Input: request.ParseInput(),
	}
}

func embeddingResponseBaidu2OpenAI(response *EmbeddingResponse) *openai.EmbeddingResponse {
	openAIEmbeddingResponse := openai.EmbeddingResponse{
		Object: "list",
		Data:   make([]openai.EmbeddingResponseItem, 0, len(response.Data)),
		Model:  "baidu-embedding",
		Usage:  response.Usage,
	}
	for _, item := range response.Data {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, openai.EmbeddingResponseItem{
			Object:    item.Object,
			Index:     item.Index,
			Embedding: item.Embedding,
		})
	}
	return &openAIEmbeddingResponse
}

func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
	var usage model.Usage
	scanner := bufio.NewScanner(resp.Body)
	helper.ConfigureScannerBuffer(scanner)
	scanner.Split(bufio.ScanLines)

	common.SetEventStreamHeaders(c)

	for scanner.Scan() {
		data := scanner.Text()
		if len(data) < 6 {
			continue
		}
		data = data[6:]

		var baiduResponse ChatStreamResponse
		err := json.Unmarshal([]byte(data), &baiduResponse)
		if err != nil {
			lg.Error("error unmarshalling stream response", zap.Error(err))
			continue
		}
		if baiduResponse.Usage.TotalTokens != 0 {
			usage.TotalTokens = baiduResponse.Usage.TotalTokens
			usage.PromptTokens = baiduResponse.Usage.PromptTokens
			usage.CompletionTokens = baiduResponse.Usage.TotalTokens - baiduResponse.Usage.PromptTokens
		}
		response := streamResponseBaidu2OpenAI(&baiduResponse)
		err = render.ObjectData(c, response)
		if err != nil {
			lg.Error("error rendering stream response", zap.Error(err))
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

func Handler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var baiduResponse ChatResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &baiduResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if baiduResponse.ErrorMsg != "" {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  baiduResponse.ErrorMsg,
				Type:     model.ErrorTypeBaidu,
				Param:    "",
				Code:     baiduResponse.ErrorCode,
				RawError: errors.New(baiduResponse.ErrorMsg),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := responseBaidu2OpenAI(&baiduResponse)
	fullTextResponse.Model = "ernie-bot"
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

func EmbeddingHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var baiduResponse EmbeddingResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &baiduResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if baiduResponse.ErrorMsg != "" {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  baiduResponse.ErrorMsg,
				Type:     model.ErrorTypeBaidu,
				Param:    "",
				Code:     baiduResponse.ErrorCode,
				RawError: errors.New(baiduResponse.ErrorMsg),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := embeddingResponseBaidu2OpenAI(&baiduResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

func GetAccessToken(apiKey string) (string, error) {
	if val, ok := baiduTokenStore.Load(apiKey); ok {
		var accessToken AccessToken
		if accessToken, ok = val.(AccessToken); ok {
			// soon this will expire
			if time.Now().Add(time.Hour).After(accessToken.ExpiresAt) {
				go func() {
					_, _ = getBaiduAccessTokenHelper(apiKey)
				}()
			}
			return accessToken.AccessToken, nil
		}
	}
	accessToken, err := getBaiduAccessTokenHelper(apiKey)
	if err != nil {
		return "", err
	}
	if accessToken == nil {
		return "", errors.New("GetAccessToken return a nil token")
	}
	return (*accessToken).AccessToken, nil
}

func getBaiduAccessTokenHelper(apiKey string) (*AccessToken, error) {
	parts := strings.Split(apiKey, "|")
	if len(parts) != 2 {
		return nil, errors.New("invalid baidu apikey")
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("https://aip.baidubce.com/oauth/2.0/token?grant_type=client_credentials&client_id=%s&client_secret=%s",
		parts[0], parts[1]), nil)
	if err != nil {
		return nil, errors.Wrap(err, "build baidu access token request")
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	res, err := client.ImpatientHTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request baidu access token")
	}
	defer res.Body.Close()

	var accessToken AccessToken
	if err = json.NewDecoder(res.Body).Decode(&accessToken); err != nil {
		return nil, errors.Wrap(err, "decode baidu access token response")
	}
	if accessToken.Error != "" {
		return nil, errors.New(accessToken.Error + ": " + accessToken.ErrorDescription)
	}
	if accessToken.AccessToken == "" {
		return nil, errors.New("getBaiduAccessTokenHelper get empty access token")
	}
	accessToken.ExpiresAt = time.Now().Add(time.Duration(accessToken.ExpiresIn) * time.Second)
	baiduTokenStore.Store(apiKey, accessToken)
	return &accessToken, nil
}
