package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	channelhelper "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

var _ adaptor.Adaptor = new(Adaptor)

const channelName = "proxy"

// Adaptor forwards requests to arbitrary upstream OpenAI-compatible endpoints while preserving billing usage.
type Adaptor struct {
	adaptor.DefaultPricingMethods
}

// Init prepares the proxy adaptor with channel metadata. No-op for proxy.
func (a *Adaptor) Init(meta *meta.Meta) {
}

// ConvertRequest forwards the incoming OpenAI-style request without modification so upstream can handle it natively.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("proxy adaptor received nil request")
	}

	// Proxy adaptor forwards the caller payload as-is so upstream can handle the request natively.
	return request, nil
}

// DoResponse writes the upstream response back to the caller and returns usage parsed from the proxied body.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	for k, v := range resp.Header {
		for _, vv := range v {
			c.Writer.Header().Set(k, vv)
		}
	}

	var responseBody bytes.Buffer
	c.Writer.WriteHeader(resp.StatusCode)
	if _, gerr := io.Copy(c.Writer, io.TeeReader(resp.Body, &responseBody)); gerr != nil {
		return nil, &relaymodel.ErrorWithStatusCode{
			StatusCode: http.StatusInternalServerError,
			Error: relaymodel.Error{
				Message:  gerr.Error(),
				RawError: errors.WithStack(gerr),
			},
		}
	}

	return proxyUsageFromResponse(responseBody.Bytes(), meta), nil
}

// GetModelList returns nil because proxy channels don't advertise specific models.
// They forward requests to upstream services where models are configured per-channel.
func (a *Adaptor) GetModelList() (models []string) {
	return nil
}

// GetChannelName returns the identifier for the proxy channel.
func (a *Adaptor) GetChannelName() string {
	return channelName
}

// GetRequestURL remove static prefix, and return the real request url to the upstream service
func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	prefix := fmt.Sprintf("/v1/oneapi/proxy/%d", meta.ChannelId)
	return meta.BaseURL + strings.TrimPrefix(meta.RequestURLPath, prefix), nil
}

// SetupRequestHeader clones caller headers to the upstream request and overrides auth-related fields.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	for k, v := range c.Request.Header {
		req.Header.Set(k, v[0])
	}

	// remove unnecessary headers
	req.Header.Del("Host")
	req.Header.Del("Content-Length")
	req.Header.Del("Accept-Encoding")
	req.Header.Del("Connection")

	// set authorization header
	req.Header.Set("Authorization", meta.APIKey)

	return nil
}

// ConvertImageRequest returns a not-implemented error because proxy channels forward raw requests.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.Errorf("not implement")
}

// ConvertClaudeRequest returns the original Claude request for pass-through proxying
func (a *Adaptor) ConvertClaudeRequest(_ *gin.Context, request *model.ClaudeRequest) (any, error) {
	return request, nil
}

// DoRequest forwards the caller request to the upstream endpoint using the shared helper.
func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return channelhelper.DoRequestHelper(a, c, meta, requestBody)
}

// proxyUsageFromResponse extracts billing usage from raw proxy responses and falls back to prompt-based usage.
// Parameters: body contains the upstream response bytes, and meta provides prompt-token and model context.
// Returns: a non-nil usage object so billed proxy relay paths do not reconcile successful requests to zero quota.
func proxyUsageFromResponse(body []byte, meta *meta.Meta) *model.Usage {
	if usage := parseProxyUsageJSON(body); hasProxyUsageTokens(usage) {
		return usage
	}
	if usage := parseProxyUsageSSE(body); hasProxyUsageTokens(usage) {
		return usage
	}

	promptTokens := 0
	if meta != nil {
		promptTokens = meta.PromptTokens
	}

	completionText := proxyCompletionTextFromJSON(body)
	if completionText == "" {
		completionText = proxyCompletionTextFromSSE(body)
	}
	if promptTokens > 0 || completionText != "" {
		completionTokens := estimateProxyTextTokens(completionText)
		return &model.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		}
	}

	return &model.Usage{}
}

// parseProxyUsageJSON extracts token usage from non-streaming OpenAI, Response API, or Claude-style JSON.
// Parameters: body is the raw upstream response body to inspect.
// Returns: parsed usage when the response carries token counters, or nil when no supported usage object exists.
func parseProxyUsageJSON(body []byte) *model.Usage {
	var envelope proxyUsageEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}

	return envelope.toUsage()
}

// parseProxyUsageSSE extracts the latest token usage block from OpenAI-compatible server-sent events.
// Parameters: body is the raw event-stream payload to inspect.
// Returns: parsed usage from the final usage-bearing event, or nil when the stream does not include usage.
func parseProxyUsageSSE(body []byte) *model.Usage {
	var usage *model.Usage
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		if parsed := parseProxyUsageJSON([]byte(data)); hasProxyUsageTokens(parsed) {
			usage = parsed
		}
	}

	return usage
}

// proxyCompletionTextFromJSON extracts assistant output text from common non-streaming proxy JSON responses.
// Parameters: body is the raw upstream response body to inspect.
// Returns: concatenated output text for fallback token estimation.
func proxyCompletionTextFromJSON(body []byte) string {
	var envelope proxyUsageEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}

	var builder strings.Builder
	for _, choice := range envelope.Choices {
		builder.WriteString(choice.Text)
		builder.WriteString(proxyStringValue(choice.Message.Content))
	}
	for _, output := range envelope.Output {
		for _, content := range output.Content {
			builder.WriteString(content.Text)
		}
	}
	for _, content := range envelope.Content {
		builder.WriteString(content.Text)
	}

	return builder.String()
}

// proxyCompletionTextFromSSE extracts streamed completion text from common OpenAI-compatible data events.
// Parameters: body is the raw event-stream payload to inspect.
// Returns: concatenated streamed output text for fallback token estimation.
func proxyCompletionTextFromSSE(body []byte) string {
	var builder strings.Builder
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var envelope proxyUsageEnvelope
		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			continue
		}
		for _, choice := range envelope.Choices {
			builder.WriteString(choice.Text)
			builder.WriteString(proxyStringValue(choice.Delta.Content))
		}
		for _, output := range envelope.Output {
			for _, content := range output.Content {
				builder.WriteString(content.Text)
			}
		}
	}

	return builder.String()
}

// hasProxyUsageTokens reports whether a parsed usage object has billable token counters.
// Parameters: usage is the parsed usage candidate.
// Returns: true when prompt, completion, total, or tool cost values are non-zero.
func hasProxyUsageTokens(usage *model.Usage) bool {
	return usage != nil && (usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.TotalTokens > 0 || usage.ToolsCost > 0)
}

// proxyStringValue converts JSON string-or-array content into a best-effort text representation.
// Parameters: value is an unmarshaled JSON field that may contain text directly or nested text parts.
// Returns: concatenated text content suitable for fallback token estimation.
func proxyStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := part["text"].(string); ok {
				builder.WriteString(text)
			}
		}
		return builder.String()
	default:
		return ""
	}
}

// proxyUsageEnvelope captures the common response fields needed to parse usage without mutating proxy payloads.
type proxyUsageEnvelope struct {
	Usage   *proxyUsageBlock     `json:"usage"`
	Choices []proxyChoice        `json:"choices"`
	Output  []proxyOutput        `json:"output"`
	Content []proxyClaudeContent `json:"content"`
}

// toUsage converts a proxy usage envelope into the shared model usage shape.
// Parameters: none.
// Returns: normalized usage, or nil when the envelope does not contain a usage block.
func (e proxyUsageEnvelope) toUsage() *model.Usage {
	if e.Usage == nil {
		return nil
	}

	promptTokens := firstPositive(e.Usage.PromptTokens, e.Usage.InputTokens)
	completionTokens := firstPositive(e.Usage.CompletionTokens, e.Usage.OutputTokens)
	totalTokens := e.Usage.TotalTokens
	if totalTokens == 0 && (promptTokens > 0 || completionTokens > 0) {
		totalTokens = promptTokens + completionTokens
	}

	return &model.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		ToolsCost:        e.Usage.ToolsCost,
	}
}

// proxyUsageBlock captures token counters used by Chat Completions, Responses, and Claude Messages APIs.
type proxyUsageBlock struct {
	PromptTokens     int   `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	InputTokens      int   `json:"input_tokens"`
	OutputTokens     int   `json:"output_tokens"`
	TotalTokens      int   `json:"total_tokens"`
	ToolsCost        int64 `json:"tools_cost"`
}

// proxyChoice captures common completion choice fields needed for fallback estimation.
type proxyChoice struct {
	Text    string       `json:"text"`
	Message proxyMessage `json:"message"`
	Delta   proxyMessage `json:"delta"`
}

// proxyMessage captures JSON content fields that may be strings or arrays.
type proxyMessage struct {
	Content any `json:"content"`
}

// proxyOutput captures Response API output items for fallback estimation.
type proxyOutput struct {
	Content []proxyOutputContent `json:"content"`
}

// proxyOutputContent captures textual Response API content for fallback estimation.
type proxyOutputContent struct {
	Text string `json:"text"`
}

// proxyClaudeContent captures Claude Messages text content for fallback estimation.
type proxyClaudeContent struct {
	Text string `json:"text"`
}

// estimateProxyTextTokens returns a conservative token estimate for proxied text when upstream omits usage.
// Parameters: text is the extracted assistant response text.
// Returns: an approximate non-negative token count based on the same character ratio used by approximate token mode.
func estimateProxyTextTokens(text string) int {
	if text == "" {
		return 0
	}

	tokens := int(float64(len(text)) * 0.38)
	if tokens == 0 {
		return 1
	}

	return tokens
}

// firstPositive returns the first positive integer from candidates.
// Parameters: values are candidate token counts ordered by preference.
// Returns: the first value greater than zero, or zero when every candidate is non-positive.
func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}

	return 0
}
