package fireworks

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/model"
)

// rerankResponse mirrors the on-the-wire shape returned by Fireworks'
// POST /v1/rerank endpoint. Fireworks emits an OpenAI-style list envelope
// with per-document scores and a standard usage block.
type rerankResponse struct {
	Object  string         `json:"object,omitempty"`
	Model   string         `json:"model,omitempty"`
	ID      string         `json:"id,omitempty"`
	Data    []rerankResult `json:"data,omitempty"`
	Usage   *model.Usage   `json:"usage,omitempty"`
	Error   *model.Error   `json:"error,omitempty"`
	Message string         `json:"message,omitempty"`
}

type rerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
	Document       string  `json:"document,omitempty"`
}

// handleRerankResponse forwards a Fireworks rerank response unchanged and
// harvests token usage for billing. Fireworks already speaks the canonical
// rerank envelope, so no translation is required.
func handleRerankResponse(c *gin.Context, resp *http.Response, promptTokens int) (*model.ErrorWithStatusCode, *model.Usage) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai_compatible.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	if cerr := resp.Body.Close(); cerr != nil {
		return openai_compatible.ErrorWrapper(cerr, "close_response_body_failed", http.StatusInternalServerError), nil
	}

	if resp.StatusCode != http.StatusOK {
		return buildRerankError(body, resp.StatusCode), nil
	}

	var parsed rerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return openai_compatible.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	if parsed.Error != nil && parsed.Error.Type != "" {
		if parsed.Error.RawError == nil && parsed.Error.Message != "" {
			parsed.Error.RawError = errors.New(parsed.Error.Message)
		}
		return &model.ErrorWithStatusCode{
			Error:      *parsed.Error,
			StatusCode: resp.StatusCode,
		}, nil
	}

	usage := parsed.Usage
	if usage == nil {
		usage = &model.Usage{}
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = promptTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, werr := c.Writer.Write(body); werr != nil {
		return openai_compatible.ErrorWrapper(werr, "write_response_body_failed", http.StatusInternalServerError), usage
	}
	return nil, usage
}

func buildRerankError(body []byte, statusCode int) *model.ErrorWithStatusCode {
	var envelope struct {
		Error   *model.Error `json:"error,omitempty"`
		Message string       `json:"message,omitempty"`
	}
	_ = json.Unmarshal(body, &envelope)

	message := strings.TrimSpace(envelope.Message)
	errType := model.ErrorType("fireworks_rerank_error")
	var code any = statusCode
	if envelope.Error != nil {
		if msg := strings.TrimSpace(envelope.Error.Message); msg != "" {
			message = msg
		}
		if t := strings.TrimSpace(string(envelope.Error.Type)); t != "" {
			errType = model.ErrorType(t)
		}
		if envelope.Error.Code != nil {
			code = envelope.Error.Code
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
