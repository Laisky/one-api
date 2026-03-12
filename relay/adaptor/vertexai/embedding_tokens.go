package vertexai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/relay/adaptor/gemini"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// EstimateGeminiEmbeddingPromptUsage counts Vertex AI Gemini embedding prompt usage via publisher models.countTokens.
// Parameters: c is the request context, meta contains Vertex AI routing metadata, and request is the embeddings payload.
// Returns: the prompt usage snapshot, whether the request is multimodal, and an API error when the estimate cannot be produced safely.
func EstimateGeminiEmbeddingPromptUsage(c *gin.Context, meta *meta.Meta, request *relaymodel.GeneralOpenAIRequest) (*relaymodel.Usage, bool, *relaymodel.ErrorWithStatusCode) {
	if meta.Config.VertexAIProjectID == "" {
		return nil, false, vertexAICountTokensError(http.StatusBadRequest, errors.New("VertexAI project ID is required but not configured for channel"))
	}

	return gemini.EstimateEmbeddingPromptUsageWithCounter(gmw.Ctx(c), request, func(_ context.Context, contents []gemini.ChatContent) (*gemini.CountTokensResponse, *relaymodel.ErrorWithStatusCode) {
		return countGeminiEmbeddingTokens(c, meta, contents)
	})
}

// countGeminiEmbeddingTokens calls the Vertex AI Gemini countTokens endpoint for embedding contents.
// Parameters: c is the request context, meta contains Vertex AI routing metadata, and contents is the normalized Gemini content list.
// Returns: the parsed countTokens response or a Gemini-shaped API error.
func countGeminiEmbeddingTokens(c *gin.Context, meta *meta.Meta, contents []gemini.ChatContent) (*gemini.CountTokensResponse, *relaymodel.ErrorWithStatusCode) {
	requestBody, err := json.Marshal(struct {
		Contents []gemini.ChatContent `json:"contents"`
	}{
		Contents: contents,
	})
	if err != nil {
		return nil, vertexAICountTokensError(http.StatusInternalServerError, errors.Wrap(err, "marshal_count_tokens_request"))
	}

	token, err := getToken(gmw.Ctx(c), meta.ChannelId, meta.Config.VertexAIADC)
	if err != nil {
		return nil, vertexAICountTokensError(http.StatusBadGateway, errors.Wrap(err, "get_vertex_access_token"))
	}

	req, err := http.NewRequestWithContext(gmw.Ctx(c), http.MethodPost, buildGeminiCountTokensURL(meta), bytes.NewReader(requestBody))
	if err != nil {
		return nil, vertexAICountTokensError(http.StatusInternalServerError, errors.Wrap(err, "new_count_tokens_request"))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	httpClient := client.HTTPClient
	if httpClient == nil {
		client.Init()
		httpClient = client.HTTPClient
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, vertexAICountTokensError(http.StatusBadGateway, errors.Wrap(err, "perform_count_tokens_request"))
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, vertexAICountTokensError(http.StatusBadGateway, errors.Wrap(err, "read_count_tokens_response"))
	}

	var countTokensResp gemini.CountTokensResponse
	if err = json.Unmarshal(responseBody, &countTokensResp); err != nil {
		return nil, vertexAICountTokensError(http.StatusBadGateway, errors.Wrap(err, "unmarshal_count_tokens_response"))
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, vertexAICountTokensAPIError(resp.StatusCode, countTokensResp.Error, responseBody)
	}
	if countTokensResp.Error != nil {
		return nil, vertexAICountTokensAPIError(http.StatusBadGateway, countTokensResp.Error, responseBody)
	}

	return &countTokensResp, nil
}

// buildGeminiCountTokensURL constructs the Vertex AI Gemini countTokens endpoint for the selected model.
// Parameters: meta contains the project, region, and model routing data.
// Returns: the absolute countTokens endpoint URL.
func buildGeminiCountTokensURL(meta *meta.Meta) string {
	adaptor := &Adaptor{}
	baseHost, location := adaptor.getDefaultHostAndLocation(meta)
	return "https://" + baseHost + "/v1/projects/" + meta.Config.VertexAIProjectID + "/locations/" + location +
		"/publishers/google/models/" + meta.ActualModelName + ":countTokens"
}

// vertexAICountTokensError converts a transport or parsing failure into a Gemini-style API error.
// Parameters: statusCode is the response status we want to surface and err is the underlying failure.
// Returns: a populated API error wrapper.
func vertexAICountTokensError(statusCode int, err error) *relaymodel.ErrorWithStatusCode {
	return &relaymodel.ErrorWithStatusCode{
		Error: relaymodel.Error{
			Message:  err.Error(),
			Type:     relaymodel.ErrorTypeGemini,
			Code:     "count_tokens_failed",
			RawError: err,
		},
		StatusCode: statusCode,
	}
}

// vertexAICountTokensAPIError converts a Vertex AI Gemini error payload into a normalized API error.
// Parameters: statusCode is the HTTP status, geminiErr is the parsed Gemini error payload, and rawBody is the raw response body.
// Returns: a Gemini-shaped API error wrapper.
func vertexAICountTokensAPIError(statusCode int, geminiErr *gemini.Error, rawBody []byte) *relaymodel.ErrorWithStatusCode {
	message := strings.TrimSpace(string(rawBody))
	code := any("count_tokens_failed")
	rawErr := errors.New(message)
	if geminiErr != nil {
		if trimmed := strings.TrimSpace(geminiErr.Message); trimmed != "" {
			message = trimmed
			rawErr = errors.New(trimmed)
		}
		if geminiErr.Code != 0 {
			code = geminiErr.Code
		}
	}

	return &relaymodel.ErrorWithStatusCode{
		Error: relaymodel.Error{
			Message:  message,
			Type:     relaymodel.ErrorTypeGemini,
			Code:     code,
			RawError: rawErr,
		},
		StatusCode: statusCode,
	}
}
