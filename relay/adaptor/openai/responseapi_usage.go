package openai

import (
	"encoding/json"
	"maps"
	"math"
	"strconv"
	"strings"

	"github.com/songquanpeng/one-api/relay/model"
)

// ResponseAPIUsage represents the usage information structure for Response API
// Response API uses different field names than Chat Completions API
type ResponseAPIUsage struct {
	InputTokens         int                             `json:"input_tokens"`
	OutputTokens        int                             `json:"output_tokens"`
	TotalTokens         int                             `json:"total_tokens"`
	InputTokensDetails  *ResponseAPIInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *ResponseAPIOutputTokensDetails `json:"output_tokens_details,omitempty"`
}

// ResponseAPIInputTokensDetails models the nested usage block returned by the OpenAI Response API.
// The schema is not stable yet (especially for web-search fields), so we keep a map of additional
// properties while still projecting the common fields into strong types.
type ResponseAPIInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
	TextTokens   int `json:"text_tokens,omitempty"`
	ImageTokens  int `json:"image_tokens,omitempty"`
	WebSearch    any `json:"web_search,omitempty"`
	additional   map[string]any
}

// ResponseAPIOutputTokensDetails models the completion-side usage details returned by the Response API.
type ResponseAPIOutputTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
	TextTokens               int `json:"text_tokens,omitempty"`
	CachedTokens             int `json:"cached_tokens,omitempty"`
	additional               map[string]any
}

func (d *ResponseAPIInputTokensDetails) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Reset existing values so the struct can be reused.
	*d = ResponseAPIInputTokensDetails{}
	if len(raw) == 0 {
		return nil
	}

	additional := make(map[string]any)
	for key, value := range raw {
		switch key {
		case "cached_tokens":
			d.CachedTokens = coerceNonNegativeInt(value)
		case "audio_tokens":
			d.AudioTokens = coerceNonNegativeInt(value)
		case "text_tokens":
			d.TextTokens = coerceNonNegativeInt(value)
		case "image_tokens":
			d.ImageTokens = coerceNonNegativeInt(value)
		case "web_search":
			d.WebSearch = value
		default:
			additional[key] = value
		}
	}

	if len(additional) > 0 {
		d.additional = additional
	}

	return nil
}

func (d ResponseAPIInputTokensDetails) MarshalJSON() ([]byte, error) {
	if d.additional == nil && d.WebSearch == nil && d.CachedTokens == 0 && d.AudioTokens == 0 && d.TextTokens == 0 && d.ImageTokens == 0 {
		return []byte("{}"), nil
	}

	raw := make(map[string]any, len(d.additional)+6)
	maps.Copy(raw, d.additional)
	if d.CachedTokens != 0 {
		raw["cached_tokens"] = d.CachedTokens
	}
	if d.AudioTokens != 0 {
		raw["audio_tokens"] = d.AudioTokens
	}
	if d.TextTokens != 0 {
		raw["text_tokens"] = d.TextTokens
	}
	if d.ImageTokens != 0 {
		raw["image_tokens"] = d.ImageTokens
	}
	if d.WebSearch != nil {
		raw["web_search"] = d.WebSearch
	}

	return json.Marshal(raw)
}

// WebSearchInvocationCount extracts the number of billable web search invocations recorded in the
// input token details payload returned by the Responses API. The upstream schema is still evolving,
// so this helper defensively inspects several possible representations (numeric counters, strings,
// or nested structures containing request entries).
func (d *ResponseAPIInputTokensDetails) WebSearchInvocationCount() int {
	if d == nil || d.WebSearch == nil {
		return 0
	}
	return extractWebSearchInvocationCount(d.WebSearch)
}

func (d *ResponseAPIOutputTokensDetails) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*d = ResponseAPIOutputTokensDetails{}
	if len(raw) == 0 {
		return nil
	}

	additional := make(map[string]any)
	for key, value := range raw {
		switch key {
		case "reasoning_tokens":
			d.ReasoningTokens = coerceNonNegativeInt(value)
		case "audio_tokens":
			d.AudioTokens = coerceNonNegativeInt(value)
		case "accepted_prediction_tokens":
			d.AcceptedPredictionTokens = coerceNonNegativeInt(value)
		case "rejected_prediction_tokens":
			d.RejectedPredictionTokens = coerceNonNegativeInt(value)
		case "text_tokens":
			d.TextTokens = coerceNonNegativeInt(value)
		case "cached_tokens":
			d.CachedTokens = coerceNonNegativeInt(value)
		default:
			additional[key] = value
		}
	}

	if len(additional) > 0 {
		d.additional = additional
	}

	return nil
}

func (d ResponseAPIOutputTokensDetails) MarshalJSON() ([]byte, error) {
	if d.additional == nil && d.ReasoningTokens == 0 && d.AudioTokens == 0 && d.AcceptedPredictionTokens == 0 && d.RejectedPredictionTokens == 0 && d.TextTokens == 0 && d.CachedTokens == 0 {
		return []byte("{}"), nil
	}

	raw := make(map[string]any, len(d.additional)+6)
	maps.Copy(raw, d.additional)
	if d.ReasoningTokens != 0 {
		raw["reasoning_tokens"] = d.ReasoningTokens
	}
	if d.AudioTokens != 0 {
		raw["audio_tokens"] = d.AudioTokens
	}
	if d.AcceptedPredictionTokens != 0 {
		raw["accepted_prediction_tokens"] = d.AcceptedPredictionTokens
	}
	if d.RejectedPredictionTokens != 0 {
		raw["rejected_prediction_tokens"] = d.RejectedPredictionTokens
	}
	if d.TextTokens != 0 {
		raw["text_tokens"] = d.TextTokens
	}
	if d.CachedTokens != 0 {
		raw["cached_tokens"] = d.CachedTokens
	}

	return json.Marshal(raw)
}

// extractWebSearchInvocationCount normalizes disparate web search metadata structures into a
// concrete invocation count. OpenAI has experimented with multiple shapes (numeric counters,
// nested maps, or arrays), so we need to support all forms without failing hard on unknown data.
func extractWebSearchInvocationCount(raw any) int {
	switch v := raw.(type) {
	case nil:
		return 0
	case int:
		if v > 0 {
			return v
		}
	case int32:
		if v > 0 {
			return int(v)
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float32:
		if v > 0 {
			return int(math.Round(float64(v)))
		}
	case float64:
		if v > 0 {
			return int(math.Round(v))
		}
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return extractWebSearchInvocationCount(f)
		}
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return extractWebSearchInvocationCount(f)
		}
	case []any:
		total := 0
		for _, item := range v {
			if count := extractWebSearchInvocationCount(item); count > 0 {
				total += count
			}
		}
		if total > 0 {
			return total
		}
		if len(v) > 0 {
			return len(v)
		}
	case map[string]any:
		// Prefer well-known counter keys first to avoid double counting.
		candidates := []string{"requests", "request_count", "count", "total_requests", "queries", "query_count", "calls", "invocations"}
		for _, key := range candidates {
			for actualKey, value := range v {
				if strings.EqualFold(actualKey, key) {
					if count := extractWebSearchInvocationCount(value); count > 0 {
						return count
					}
				}
			}
		}
		// As a defensive fallback, inspect remaining values and return the first positive count.
		for _, value := range v {
			if count := extractWebSearchInvocationCount(value); count > 0 {
				return count
			}
		}
	}
	return 0
}

func (d *ResponseAPIInputTokensDetails) toModel() *model.UsagePromptTokensDetails {
	if d == nil {
		return nil
	}

	details := &model.UsagePromptTokensDetails{
		CachedTokens: d.CachedTokens,
		AudioTokens:  d.AudioTokens,
		TextTokens:   d.TextTokens,
		ImageTokens:  d.ImageTokens,
	}
	return details
}

func (d *ResponseAPIOutputTokensDetails) toModel() *model.UsageCompletionTokensDetails {
	if d == nil {
		return nil
	}

	return &model.UsageCompletionTokensDetails{
		ReasoningTokens:          d.ReasoningTokens,
		AudioTokens:              d.AudioTokens,
		AcceptedPredictionTokens: d.AcceptedPredictionTokens,
		RejectedPredictionTokens: d.RejectedPredictionTokens,
		TextTokens:               d.TextTokens,
		CachedTokens:             d.CachedTokens,
	}
}

func newResponseAPIInputTokensDetailsFromModel(details *model.UsagePromptTokensDetails) *ResponseAPIInputTokensDetails {
	if details == nil {
		return nil
	}

	converted := &ResponseAPIInputTokensDetails{
		CachedTokens: details.CachedTokens,
		AudioTokens:  details.AudioTokens,
		TextTokens:   details.TextTokens,
		ImageTokens:  details.ImageTokens,
	}
	return converted
}

func newResponseAPIOutputTokensDetailsFromModel(details *model.UsageCompletionTokensDetails) *ResponseAPIOutputTokensDetails {
	if details == nil {
		return nil
	}

	return &ResponseAPIOutputTokensDetails{
		ReasoningTokens:          details.ReasoningTokens,
		AudioTokens:              details.AudioTokens,
		AcceptedPredictionTokens: details.AcceptedPredictionTokens,
		RejectedPredictionTokens: details.RejectedPredictionTokens,
		TextTokens:               details.TextTokens,
		CachedTokens:             details.CachedTokens,
	}
}

func coerceNonNegativeInt(value any) int {
	const maxInt = int(^uint(0) >> 1)

	switch v := value.(type) {
	case nil:
		return 0
	case int:
		if v < 0 {
			return 0
		}
		return v
	case int8:
		if v < 0 {
			return 0
		}
		return int(v)
	case int16:
		if v < 0 {
			return 0
		}
		return int(v)
	case int32:
		if v < 0 {
			return 0
		}
		return int(v)
	case int64:
		if v < 0 {
			return 0
		}
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		if v > uint64(maxInt) {
			return maxInt
		}
		return int(v)
	case float32:
		if v < 0 {
			return 0
		}
		return int(v)
	case float64:
		if v < 0 {
			return 0
		}
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			if i < 0 {
				return 0
			}
			return int(i)
		}
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0
		}
		if i, err := strconv.ParseFloat(s, 64); err == nil {
			if i < 0 {
				return 0
			}
			return int(i)
		}
	}

	return 0
}

// ToModelUsage converts ResponseAPIUsage to model.Usage for compatibility.
func (r *ResponseAPIUsage) ToModelUsage() *model.Usage {
	if r == nil {
		return nil
	}

	usage := &model.Usage{
		PromptTokens:     r.InputTokens,
		CompletionTokens: r.OutputTokens,
		TotalTokens:      r.TotalTokens,
	}
	usage.PromptTokensDetails = r.InputTokensDetails.toModel()
	usage.CompletionTokensDetails = r.OutputTokensDetails.toModel()
	return usage
}

// FromModelUsage converts model.Usage to ResponseAPIUsage for compatibility.
func (r *ResponseAPIUsage) FromModelUsage(usage *model.Usage) *ResponseAPIUsage {
	if usage == nil {
		return nil
	}

	converted := &ResponseAPIUsage{
		InputTokens:         usage.PromptTokens,
		OutputTokens:        usage.CompletionTokens,
		TotalTokens:         usage.TotalTokens,
		InputTokensDetails:  newResponseAPIInputTokensDetailsFromModel(usage.PromptTokensDetails),
		OutputTokensDetails: newResponseAPIOutputTokensDetailsFromModel(usage.CompletionTokensDetails),
	}

	return converted
}
