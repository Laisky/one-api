package cohere

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

type RerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            *int     `json:"top_n,omitempty"`
	MaxTokensPerDoc *int     `json:"max_tokens_per_doc,omitempty"`
	Priority        *int     `json:"priority,omitempty"`
}

type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type RerankTokenUsage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CachedTokens float64 `json:"cached_tokens,omitempty"`
}

type RerankBilledUnits struct {
	SearchUnits int `json:"search_units"`
}

type RerankAPIVersion struct {
	Version        string `json:"version"`
	IsExperimental bool   `json:"is_experimental"`
}

type RerankMeta struct {
	APIVersion  *RerankAPIVersion  `json:"api_version,omitempty"`
	BilledUnits *RerankBilledUnits `json:"billed_units,omitempty"`
	Tokens      *RerankTokenUsage  `json:"tokens,omitempty"`
}

type RerankResponse struct {
	Object   string         `json:"object,omitempty"`
	Model    string         `json:"model,omitempty"`
	ID       string         `json:"id,omitempty"`
	Results  []RerankResult `json:"results,omitempty"`
	Meta     *RerankMeta    `json:"meta,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	Usage    *model.Usage   `json:"usage,omitempty"`
	Message  string         `json:"message,omitempty"`
}

type RerankErrorInfo struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

type RerankErrorResponse struct {
	Message string           `json:"message"`
	Error   *RerankErrorInfo `json:"error"`
}

// ConvertRerankRequest normalizes a rerank request into Cohere's payload format.
func ConvertRerankRequest(request model.RerankRequest) (*RerankRequest, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, errors.New("rerank query is empty")
	}
	if len(request.Documents) == 0 {
		return nil, errors.New("rerank documents are empty")
	}

	return &RerankRequest{
		Model:           request.Model,
		Query:           query,
		Documents:       append([]string(nil), request.Documents...),
		TopN:            request.TopN,
		MaxTokensPerDoc: request.MaxTokensPerDoc,
		Priority:        request.Priority,
	}, nil
}

// RerankHandler adapts Cohere rerank responses to the unified API response format.
func RerankHandler(c *gin.Context, resp *http.Response, meta *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		return openai.ErrorWrapper(closeErr, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	if resp.StatusCode != http.StatusOK {
		return buildRerankError(body, resp.StatusCode), nil
	}

	var cohereResponse RerankResponse
	if err := json.Unmarshal(body, &cohereResponse); err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	usage := deriveRerankUsage(meta, &cohereResponse)
	cohereResponse.Usage = usage
	if meta != nil {
		cohereResponse.Model = meta.ActualModelName
	}
	if cohereResponse.Object == "" {
		cohereResponse.Object = "cohere.rerank"
	}

	responseBytes, err := json.Marshal(cohereResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = c.Writer.Write(responseBytes); err != nil {
		return openai.ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError), usage
	}

	return nil, usage
}

func buildRerankError(body []byte, statusCode int) *model.ErrorWithStatusCode {
	var errResp RerankErrorResponse
	_ = json.Unmarshal(body, &errResp)

	message := strings.TrimSpace(errResp.Message)
	errType := model.ErrorType("cohere_rerank_error")
	var code any = statusCode
	if errResp.Error != nil {
		if strings.TrimSpace(errResp.Error.Message) != "" {
			message = strings.TrimSpace(errResp.Error.Message)
		}
		if strings.TrimSpace(errResp.Error.Type) != "" {
			errType = model.ErrorType(errResp.Error.Type)
		}
		if errResp.Error.Code != nil {
			code = errResp.Error.Code
		}
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}

	return &model.ErrorWithStatusCode{
		Error: model.Error{
			Message:  message,
			Type:     errType,
			Code:     code,
			RawError: errors.New(message),
		},
		StatusCode: statusCode,
	}
}

func deriveRerankUsage(meta *meta.Meta, resp *RerankResponse) *model.Usage {
	promptTokens := 0
	if meta != nil {
		promptTokens = meta.PromptTokens
	}
	usage := &model.Usage{
		PromptTokens: promptTokens,
		TotalTokens:  promptTokens,
	}

	if resp != nil && resp.Meta != nil && resp.Meta.Tokens != nil {
		tokens := resp.Meta.Tokens
		if tokens.InputTokens > 0 {
			usage.PromptTokens = tokens.InputTokens
		}
		if tokens.OutputTokens > 0 {
			usage.CompletionTokens = tokens.OutputTokens
		}
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		if tokens.CachedTokens > 0 {
			usage.PromptTokensDetails = &model.UsagePromptTokensDetails{
				CachedTokens: int(math.Round(tokens.CachedTokens)),
			}
		}
	}

	return usage
}
