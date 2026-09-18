package typesafe

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// maxResponseBytes bounds buffering of native answers and upstream error bodies.
const maxResponseBytes = 8 << 20

// Response retains native JSON unchanged until billing has completed.
type Response struct {
	Body       []byte
	StatusCode int
	RetryAfter string
}

// Write commits the buffered native response, including provider retry guidance.
func (r *Response) Write(c *gin.Context) {
	if r.RetryAfter != "" {
		c.Header("Retry-After", r.RetryAfter)
	}
	c.Data(r.StatusCode, "application/json", r.Body)
}

// EstimatedUsage labels an uncertain paid attempt rather than inventing free work.
func EstimatedUsage(reason string) *model.Usage {
	return &model.Usage{PromptTokens: AdmissionInputTokens, TotalTokens: AdmissionInputTokens, BillingEstimateReason: reason}
}

// ReadResponse closes the body, extracts billing evidence and buffers native JSON.
// A verified input receipt remains billable even when the answer shape is invalid.
func (a *Adaptor) ReadResponse(c *gin.Context, response *http.Response, _ *meta.Meta) (*Response, *model.Usage, *model.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)
	usage := EstimatedUsage("typesafe_missing_or_invalid_receipt")
	if response == nil || response.Body == nil {
		return nil, usage, openai.ErrorWrapper(errors.New("empty TypeSafe upstream response"), "typesafe_empty_response", http.StatusBadGateway)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			lg.Warn("close TypeSafe response body failed", zap.Error(err))
		}
	}()
	if IsAdmissionRejection(response.StatusCode) {
		usage = &model.Usage{}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return nil, usage, openai.ErrorWrapper(errors.New("incomplete or oversized TypeSafe response"), "typesafe_response_read_failed", http.StatusBadGateway)
	}
	result := &Response{Body: body, StatusCode: response.StatusCode, RetryAfter: response.Header.Get("Retry-After")}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode < 400 {
			return nil, usage, openai.ErrorWrapper(errors.New("unexpected TypeSafe success status"), "typesafe_invalid_status", http.StatusBadGateway)
		}
		if !json.Valid(body) {
			result = nil
		}
		return result, usage, openai.ErrorWrapper(errors.Errorf("TypeSafe returned HTTP %d", response.StatusCode), "typesafe_upstream_error", response.StatusCode)
	}
	fields, err := decodeObject(body)
	if err != nil {
		return nil, usage, openai.ErrorWrapper(err, "typesafe_invalid_response", http.StatusBadGateway)
	}
	receipt, err := decodeReceipt(fields["usage"])
	if receipt != nil {
		usage = receipt
	}
	if err != nil {
		return nil, usage, openai.ErrorWrapper(err, "typesafe_invalid_usage", http.StatusBadGateway)
	}
	var upstreamModel string
	if err := json.Unmarshal(fields["model"], &upstreamModel); err != nil || upstreamModel == "" {
		return nil, usage, openai.ErrorWrapper(errors.New("TypeSafe response model is missing"), "typesafe_invalid_response", http.StatusBadGateway)
	}
	answers, err := decodeObject(fields["answers"])
	if err != nil || len(answers) == 0 {
		return nil, usage, openai.ErrorWrapper(errors.New("TypeSafe response answers are missing"), "typesafe_invalid_response", http.StatusBadGateway)
	}
	if value, ok := c.Get(ctxkey.ConvertedRequest); ok {
		if request, ok := value.(*Request); ok {
			if err := validateAnswerIDs(answers, request); err != nil {
				return nil, usage, openai.ErrorWrapper(err, "typesafe_invalid_response", http.StatusBadGateway)
			}
		}
	}
	return result, usage, nil
}

// decodeReceipt accepts complete nonnegative integer counters without float coercion.
func decodeReceipt(raw json.RawMessage) (*model.Usage, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return nil, errors.Wrap(err, "decode TypeSafe usage")
	}
	var input, output *int
	if err := json.Unmarshal(fields["input_tokens"], &input); err != nil || input == nil || *input <= 0 {
		return nil, errors.New("TypeSafe input_tokens must be a positive integer")
	}
	usage := &model.Usage{PromptTokens: *input, TotalTokens: *input}
	if err := json.Unmarshal(fields["output_tokens"], &output); err != nil || output == nil || *output < 0 {
		return usage, errors.New("TypeSafe output_tokens must be a nonnegative integer")
	}
	maxInt := int(^uint(0) >> 1)
	if *output > maxInt-*input {
		return usage, errors.New("TypeSafe total token counter overflows")
	}
	return &model.Usage{PromptTokens: *input, CompletionTokens: *output, TotalTokens: *input + *output}, nil
}

// validateAnswerIDs ensures the provider answered exactly the requested questions.
func validateAnswerIDs(answers map[string]json.RawMessage, request *Request) error {
	if len(answers) != len(request.Questions) {
		return errors.New("TypeSafe answer count does not match the request")
	}
	for id, question := range request.Questions {
		answer, err := decodeObject(answers[id])
		if err != nil {
			return errors.New("TypeSafe answer ID is missing or malformed")
		}
		var kind string
		if err := json.Unmarshal(answer["type"], &kind); err != nil || kind != question.Type {
			return errors.New("TypeSafe answer type does not match its question")
		}
	}
	return nil
}
