package deepinfra

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

type completionChoice struct {
	Text string `json:"text"`
}

type completionEnvelope struct {
	Choices []completionChoice `json:"choices"`
	Usage   *model.Usage       `json:"usage,omitempty"`
	Error   *model.Error       `json:"error,omitempty"`
}

// handleCompletionResponse preserves the legacy OpenAI choices[].text envelope
// while extracting or synthesizing usage for one-api billing.
func handleCompletionResponse(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
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

	var parsed completionEnvelope
	if err := json.Unmarshal(body, &parsed); err != nil {
		return openai_compatible.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		if parsed.Error.RawError == nil {
			parsed.Error.RawError = errors.New(parsed.Error.Message)
		}
		return &model.ErrorWithStatusCode{Error: *parsed.Error, StatusCode: resp.StatusCode}, nil
	}
	if len(parsed.Choices) == 0 {
		return openai_compatible.ErrorWrapper(errors.New("completion response has no choices"), "no_choices_in_response", http.StatusBadGateway), nil
	}

	usage := finalizeCompletionUsage(parsed.Usage, parsed.Choices, promptTokens, modelName)

	// Add synthesized usage without losing provider-specific top-level or choice fields.
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return openai_compatible.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	usageJSON, err := json.Marshal(usage)
	if err != nil {
		return openai_compatible.ErrorWrapper(err, "marshal_usage_failed", http.StatusInternalServerError), nil
	}
	payload["usage"] = usageJSON
	rendered, err := json.Marshal(payload)
	if err != nil {
		return openai_compatible.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}

	c.Data(resp.StatusCode, "application/json", rendered)
	return nil, usage
}

// handleCompletionStream forwards legacy completion SSE events unchanged and
// accumulates choices[].text for fallback usage accounting.
func handleCompletionStream(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	if c == nil {
		return openai_compatible.ErrorWrapper(errors.New("gin context is nil"), "invalid_context", http.StatusInternalServerError), nil
	}
	if resp == nil || resp.Body == nil {
		return openai_compatible.ErrorWrapper(errors.New("upstream response is nil"), "invalid_response", http.StatusInternalServerError), nil
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
	}
	c.Status(resp.StatusCode)

	reader := bufio.NewReader(resp.Body)
	usage := &model.Usage{}
	var completionText strings.Builder
	var writeErr error

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			parseCompletionStreamLine(line, usage, &completionText)
			if writeErr == nil {
				if _, err := c.Writer.Write(line); err != nil {
					writeErr = err
				} else if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return openai_compatible.ErrorWrapper(readErr, "read_stream_failed", http.StatusInternalServerError), usage
			}
			break
		}
	}

	finalizeCompletionUsage(usage, []completionChoice{{Text: completionText.String()}}, promptTokens, modelName)
	if writeErr != nil {
		return openai_compatible.ErrorWrapper(writeErr, "write_stream_failed", http.StatusInternalServerError), usage
	}
	return nil, usage
}

func parseCompletionStreamLine(line []byte, usage *model.Usage, completionText *strings.Builder) {
	trimmed := strings.TrimSpace(string(line))
	if !strings.HasPrefix(trimmed, "data:") {
		return
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "" || payload == "[DONE]" {
		return
	}

	var chunk completionEnvelope
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return
	}
	for _, choice := range chunk.Choices {
		completionText.WriteString(choice.Text)
	}
	if chunk.Usage != nil {
		*usage = *chunk.Usage
	}
}

func finalizeCompletionUsage(usage *model.Usage, choices []completionChoice, promptTokens int, modelName string) *model.Usage {
	if usage == nil {
		usage = &model.Usage{}
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = promptTokens
	}
	if usage.CompletionTokens == 0 {
		for _, choice := range choices {
			usage.CompletionTokens += openai_compatible.CountTokenText(choice.Text, modelName)
		}
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	usage.NormalizeCachedTokens()
	usage.NormalizeCacheWriteTokens()
	return usage
}
