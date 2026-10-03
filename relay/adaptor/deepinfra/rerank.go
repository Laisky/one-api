package deepinfra

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

const (
	rerankDocumentsContextKey = "deepinfra_rerank_documents"
	rerankTopNContextKey      = "deepinfra_rerank_top_n"
)

type upstreamRerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type upstreamRerankResponse struct {
	Scores []float64 `json:"scores"`
}

type canonicalRerankResponse struct {
	Object string                  `json:"object"`
	Model  string                  `json:"model,omitempty"`
	Data   []canonicalRerankResult `json:"data"`
	Usage  *model.Usage            `json:"usage,omitempty"`
}

type canonicalRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
	Document       string  `json:"document,omitempty"`
}

// ConvertRerankRequest converts one-api's canonical rerank DTO to DeepInfra's
// model-native {query, documents} request and preserves response shaping state.
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, request *model.RerankRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if strings.TrimSpace(request.Query) == "" {
		return nil, errors.New("rerank query is empty")
	}
	if len(request.Documents) == 0 {
		return nil, errors.New("rerank documents are empty")
	}
	if request.MaxTokensPerDoc != nil {
		return nil, errors.New("DeepInfra rerank does not support max_tokens_per_doc")
	}
	if request.Priority != nil {
		return nil, errors.New("DeepInfra rerank does not support priority")
	}

	topN := len(request.Documents)
	if request.TopN != nil {
		if *request.TopN <= 0 {
			return nil, errors.New("top_n must be greater than zero")
		}
		if *request.TopN > len(request.Documents) {
			return nil, errors.New("top_n cannot exceed the number of documents")
		}
		topN = *request.TopN
	}

	documents := append([]string(nil), request.Documents...)
	if c != nil {
		c.Set(rerankDocumentsContextKey, documents)
		c.Set(rerankTopNContextKey, topN)
	}
	return &upstreamRerankRequest{
		Query:     request.Query,
		Documents: documents,
	}, nil
}

// handleRerankResponse converts DeepInfra's score vector into the canonical
// sorted rerank list while retaining original document indexes.
func handleRerankResponse(c *gin.Context, resp *http.Response, modelName string, promptTokens int) (*model.ErrorWithStatusCode, *model.Usage) {
	if c == nil {
		return openai_compatible.ErrorWrapper(errors.New("gin context is nil"), "invalid_context", http.StatusInternalServerError), nil
	}
	if resp == nil || resp.Body == nil {
		return openai_compatible.ErrorWrapper(errors.New("upstream response is nil"), "invalid_response", http.StatusInternalServerError), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai_compatible.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		return openai_compatible.ErrorWrapper(closeErr, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return buildDeepInfraError(body, resp.StatusCode), nil
	}

	var upstream upstreamRerankResponse
	if err := json.Unmarshal(body, &upstream); err != nil {
		return openai_compatible.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	documents, ok := c.Get(rerankDocumentsContextKey)
	if !ok {
		return openai_compatible.ErrorWrapper(errors.New("rerank documents are missing from request context"), "missing_rerank_context", http.StatusInternalServerError), nil
	}
	documentList, ok := documents.([]string)
	if !ok {
		return openai_compatible.ErrorWrapper(errors.New("rerank documents have an invalid context type"), "invalid_rerank_context", http.StatusInternalServerError), nil
	}
	if len(upstream.Scores) != len(documentList) {
		return openai_compatible.ErrorWrapper(
			errors.Errorf("DeepInfra returned %d scores for %d documents", len(upstream.Scores), len(documentList)),
			"invalid_rerank_response",
			http.StatusBadGateway,
		), nil
	}

	results := make([]canonicalRerankResult, 0, len(documentList))
	for index, score := range upstream.Scores {
		results = append(results, canonicalRerankResult{
			Index:          index,
			RelevanceScore: score,
			Document:       documentList[index],
		})
	}
	sort.SliceStable(results, func(left, right int) bool {
		return results[left].RelevanceScore > results[right].RelevanceScore
	})

	topN := len(results)
	if value, exists := c.Get(rerankTopNContextKey); exists {
		if requestedTopN, valid := value.(int); valid && requestedTopN > 0 && requestedTopN < topN {
			topN = requestedTopN
		}
	}
	results = results[:topN]

	usage := &model.Usage{
		PromptTokens: promptTokens,
		TotalTokens:  promptTokens,
	}
	c.JSON(resp.StatusCode, canonicalRerankResponse{
		Object: "list",
		Model:  modelName,
		Data:   results,
		Usage:  usage,
	})
	return nil, usage
}

// buildDeepInfraError normalizes DeepInfra's error and validation envelopes.
func buildDeepInfraError(body []byte, statusCode int) *model.ErrorWithStatusCode {
	var envelope struct {
		Error   *model.Error    `json:"error,omitempty"`
		Message string          `json:"message,omitempty"`
		Detail  json.RawMessage `json:"detail,omitempty"`
	}
	_ = json.Unmarshal(body, &envelope)

	message := strings.TrimSpace(envelope.Message)
	errorType := model.ErrorType("deepinfra_error")
	var code any = statusCode
	if envelope.Error != nil {
		if value := strings.TrimSpace(envelope.Error.Message); value != "" {
			message = value
		}
		if value := strings.TrimSpace(string(envelope.Error.Type)); value != "" {
			errorType = model.ErrorType(value)
		}
		if envelope.Error.Code != nil {
			code = envelope.Error.Code
		}
	}
	if message == "" {
		message = detailMessage(envelope.Detail)
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}

	return &model.ErrorWithStatusCode{
		Error: model.Error{
			Message:  message,
			Type:     errorType,
			Code:     code,
			RawError: errors.New(message),
		},
		StatusCode: statusCode,
	}
}

// detailMessage renders FastAPI-style detail strings or validation issue arrays.
func detailMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var text string
	if json.Unmarshal(raw, &text) == nil {
		return strings.TrimSpace(text)
	}

	var issues []struct {
		Message string `json:"msg"`
		Type    string `json:"type"`
	}
	if json.Unmarshal(raw, &issues) == nil {
		messages := make([]string, 0, len(issues))
		for _, issue := range issues {
			message := strings.TrimSpace(issue.Message)
			if message == "" {
				message = strings.TrimSpace(issue.Type)
			}
			if message != "" {
				messages = append(messages, message)
			}
		}
		return strings.Join(messages, "; ")
	}

	return strings.TrimSpace(fmt.Sprintf("%s", raw))
}
