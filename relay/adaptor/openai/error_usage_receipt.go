package openai

import (
	"bytes"
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// PaidErrorReceipt implements adaptor.PaidErrorReceiptReader. OpenAI documents
// no usage in its error schema, but a Responses envelope can describe a failed
// generation, and a non-2xx body that nevertheless carries an explicit receipt
// is evidence of paid work that must never be refunded or replayed. Only the
// native OpenAI and Azure OpenAI wire contracts are trusted; other channel
// types served by this adaptor keep the generic retained-hold path. A failed
// Responses envelope's executed web searches are recorded on c exactly as the
// 2xx handlers record them. Parameters: c receives tool invocation counts, m
// identifies the channel, statusCode is the upstream status, and body is the
// bounded error body. Returns: the receipt usage, or nil.
func (a *Adaptor) PaidErrorReceipt(c *gin.Context, m *meta.Meta, statusCode int, body []byte) *model.Usage {
	if m == nil || statusCode/100 == 2 {
		return nil
	}
	switch m.ChannelType {
	case channeltype.OpenAI, channeltype.Azure:
	default:
		return nil
	}
	usage := ParseErrorUsageReceipt(body)
	if usage != nil && !deepseekcompat.UsesDeepSeekToolReceiptContract(m) {
		recordErrorReceiptWebSearchCalls(c, body)
	}
	return usage
}

// recordErrorReceiptWebSearchCalls counts the chargeable web_search_call
// actions a failed Responses envelope lists (root object or response
// envelope) with the same rules as the 2xx handlers, so a paid error settles
// its executed searches. Parameters: c receives the count and body is the
// bounded error body. Returns: nothing; bodies without a Responses output are
// ignored.
func recordErrorReceiptWebSearchCalls(c *gin.Context, body []byte) {
	if c == nil {
		return
	}
	var envelope struct {
		ResponseAPIResponse
		Response *ResponseAPIResponse `json:"response"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return
	}
	target := &envelope.ResponseAPIResponse
	if envelope.Response != nil && len(envelope.Response.Output) > 0 {
		target = envelope.Response
	}
	calls, _ := deriveWebSearchInvocationCount(countWebSearchSearchActions(target.Output), target.Usage)
	recordWebSearchInvocationCount(c, calls)
}

// ParseErrorUsageReceipt extracts an explicit usage receipt from an OpenAI-wire
// error body: a root usage object (Chat Completions style or a Responses
// object) or a Responses envelope's response.usage. A body without such an
// object, with only zero counters, or above adaptor.MaxPaidErrorReceiptBytes
// proves no billable work. Parameters: body is the raw error body. Returns: the
// normalized usage (service tier and cache dimensions included), or nil.
func ParseErrorUsageReceipt(body []byte) *model.Usage {
	if len(body) == 0 || len(body) > adaptor.MaxPaidErrorReceiptBytes {
		return nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil
	}
	if !isUsageObject(root["usage"]) {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(root["response"], &envelope); err != nil || !isUsageObject(envelope["usage"]) {
			return nil
		}
	}
	usage := model.ParseResponseUsage(body, 0, nil)
	if usage == nil || (usage.PromptTokens <= 0 && usage.CompletionTokens <= 0 && usage.ToolsCost <= 0 &&
		usage.CacheWrite5mTokens <= 0 && usage.CacheWrite1hTokens <= 0 && usage.CacheWrite12hTokens <= 0) {
		return nil
	}
	return usage
}

// isUsageObject reports whether raw is a JSON object rather than absent, null
// or a scalar. Parameters: raw is one decoded field. Returns: true for an object.
func isUsageObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 1 && trimmed[0] == '{'
}
