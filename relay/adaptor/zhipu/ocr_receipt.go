package zhipu

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// GLM-OCR billing evidence (verified 2026-10-05):
//   - BigModel (open.bigmodel.cn, CNY) bills GLM-OCR per token, 0.2 CNY per 1M
//     input and output tokens: https://docs.bigmodel.cn/cn/guide/start/pricing.md
//     and https://docs.bigmodel.cn/cn/guide/models/vlm/glm-ocr.md
//   - Z.ai (api.z.ai, USD) bills GLM-OCR per token, 0.03 USD per 1M input and
//     output tokens: https://docs.z.ai/guides/overview/pricing
//   - Both layout_parsing references document usage.prompt_tokens,
//     usage.completion_tokens, usage.total_tokens and
//     usage.prompt_tokens_details.cached_tokens, plus data_info.num_pages:
//     https://docs.bigmodel.cn/api-reference/模型-api/文档解析.md and
//     https://docs.z.ai/api-reference/tools/layout-parsing.md

// OCRHandler forwards a native layout_parsing response for the chat-completions
// OCR route and returns its token usage. Missing or unusable counters are
// returned as a labelled estimate, never as an authoritative zero-token receipt.
// Parameters: c is the client request, resp is the upstream response, and the
// model name is unused. Returns: an API error, if any, and the usage evidence.
func OCRHandler(c *gin.Context, resp *http.Response, _ string) (*model.ErrorWithStatusCode, *model.Usage) {
	receipt, apiErr := forwardOCRResponse(c, resp)
	return apiErr, receipt.UsageOrEstimate()
}

// forwardOCRResponse reads the upstream layout_parsing body once, extracts its
// typed billing receipt, and forwards the body to the client unchanged. The
// receipt is always returned, including when delivery to the client fails after
// the provider accepted the work, so the caller can settle exactly once.
// Parameters: c is the client request and resp is the upstream response.
// Returns: the receipt evidence and an API error when reading or delivery failed.
func forwardOCRResponse(c *gin.Context, resp *http.Response) (*model.OCRReceipt, *model.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)
	body, readErr := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil {
		lg.Warn("close upstream OCR response body failed", zap.Error(closeErr))
	}
	if readErr != nil {
		return model.UnreadableOCRReceipt(), openai.ErrorWrapper(
			errors.Wrap(readErr, "read upstream OCR response"), "read_response_body_failed", http.StatusInternalServerError)
	}

	receipt := ParseOCRReceipt(body)
	lg.Debug("parsed upstream OCR receipt",
		zap.Bool("usage_valid", receipt.UsageProblem == ""),
		zap.String("usage_problem", receipt.UsageProblem),
		zap.Int("pages", receipt.Pages),
		zap.String("pages_problem", receipt.PagesProblem))
	if receipt.UsageProblem == model.OCRReceiptUnreadable {
		return receipt, openai.ErrorWrapper(
			errors.New("upstream OCR response is not a JSON object"), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := c.Writer.Write(body); err != nil {
		return receipt, openai.ErrorWrapper(
			errors.Wrap(err, "deliver OCR response to client"), "write_response_body_failed", http.StatusInternalServerError)
	}
	return receipt, nil
}

// ParseOCRReceipt validates the billing counters of a layout_parsing body.
// Counters must be nonnegative JSON integers within the receipt bounds, the
// optional total must equal prompt plus completion, and cached tokens may not
// exceed prompt tokens. Each dimension that fails carries its own problem label.
// Parameters: body is the raw upstream response. Returns: the typed receipt.
func ParseOCRReceipt(body []byte) *model.OCRReceipt {
	var envelope struct {
		Usage    json.RawMessage `json:"usage"`
		DataInfo json.RawMessage `json:"data_info"`
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &envelope) != nil {
		return model.UnreadableOCRReceipt()
	}
	receipt := &model.OCRReceipt{}
	receipt.Usage, receipt.UsageProblem = parseOCRUsage(envelope.Usage)
	receipt.Pages, receipt.PagesProblem = parseOCRPages(envelope.DataInfo)
	return receipt
}

// parseOCRUsage validates the usage object of an OCR receipt.
// Parameters: raw is the JSON usage value. Returns: usage, or nil and a problem label.
func parseOCRUsage(raw json.RawMessage) (*model.Usage, string) {
	if isAbsentJSON(raw) {
		return nil, model.OCRReceiptMissing
	}
	var wire struct {
		PromptTokens        json.RawMessage `json:"prompt_tokens"`
		CompletionTokens    json.RawMessage `json:"completion_tokens"`
		TotalTokens         json.RawMessage `json:"total_tokens"`
		PromptTokensDetails *struct {
			CachedTokens json.RawMessage `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	if bytes.TrimSpace(raw)[0] != '{' || json.Unmarshal(raw, &wire) != nil {
		return nil, model.OCRReceiptInvalid
	}
	prompt, problem := parseOCRCounter(wire.PromptTokens, model.MaxOCRReceiptTokens)
	if problem != "" {
		return nil, problem
	}
	completion, problem := parseOCRCounter(wire.CompletionTokens, model.MaxOCRReceiptTokens)
	if problem != "" {
		return nil, problem
	}
	usage := &model.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion}
	if !isAbsentJSON(wire.TotalTokens) {
		total, problem := parseOCRCounter(wire.TotalTokens, 2*model.MaxOCRReceiptTokens)
		if problem != "" {
			return nil, problem
		}
		if total != usage.TotalTokens {
			return nil, model.OCRReceiptInconsistent
		}
	}
	if details := wire.PromptTokensDetails; details != nil && !isAbsentJSON(details.CachedTokens) {
		cached, problem := parseOCRCounter(details.CachedTokens, model.MaxOCRReceiptTokens)
		if problem != "" {
			return nil, problem
		}
		if cached > prompt {
			return nil, model.OCRReceiptInconsistent
		}
		if cached > 0 {
			usage.PromptTokensDetails = &model.UsagePromptTokensDetails{CachedTokens: cached}
		}
	}
	return usage, ""
}

// parseOCRPages validates data_info.num_pages of an OCR receipt.
// Parameters: raw is the JSON data_info value. Returns: pages, or zero and a problem label.
func parseOCRPages(raw json.RawMessage) (int, string) {
	if isAbsentJSON(raw) {
		return 0, model.OCRReceiptMissing
	}
	var wire struct {
		NumPages json.RawMessage `json:"num_pages"`
	}
	if bytes.TrimSpace(raw)[0] != '{' || json.Unmarshal(raw, &wire) != nil {
		return 0, model.OCRReceiptInvalid
	}
	return parseOCRCounter(wire.NumPages, model.MaxOCRReceiptPages)
}

// parseOCRCounter parses one receipt counter as an exact nonnegative JSON integer.
// Fractions, exponents, strings, negative numbers and other JSON types are invalid;
// values above limit, including values beyond int64, overflow.
// Parameters: raw is the JSON value and limit is the inclusive bound.
// Returns: the counter, or zero and a problem label.
func parseOCRCounter(raw json.RawMessage, limit int) (int, string) {
	if isAbsentJSON(raw) {
		return 0, model.OCRReceiptMissing
	}
	text := string(bytes.TrimSpace(raw))
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return 0, model.OCRReceiptInvalid
		}
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		var numErr *strconv.NumError
		if errors.As(err, &numErr) && errors.Is(numErr.Err, strconv.ErrRange) {
			return 0, model.OCRReceiptOverflow
		}
		return 0, model.OCRReceiptInvalid
	}
	if value > uint64(limit) {
		return 0, model.OCRReceiptOverflow
	}
	return int(value), ""
}

// isAbsentJSON reports whether raw is an omitted or explicit null JSON value.
func isAbsentJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
