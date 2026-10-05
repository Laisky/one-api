package controller

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/tooling"
)

// claudeServerToolDefaultReservedUses bounds the paid-tool allowance reserved
// for a declaration without max_uses. Anthropic documents no default cap, so the
// gateway reserves this many invocations as a conservative floor. The floor is
// retained only when the upstream receipt is missing or incomplete; a complete
// receipt settles the actual invocation count and refunds the rest.
const claudeServerToolDefaultReservedUses = int64(10)

// claudeCapabilityRootFields are the root fields of a Claude Messages body that
// can opt into provider-executed capabilities.
var claudeCapabilityRootFields = []string{"tools", "mcp_servers", "container"}

// claudeToolPolicyFields are the tool-object fields admission classifies.
var claudeToolPolicyFields = []string{"type", "max_uses"}

// resetClaudeToolAttemptState clears tool counters a previous attempt may have
// left on a reused gin context, so only this attempt's receipts are billed.
// Parameters: c is the request context. Returns: nothing.
func resetClaudeToolAttemptState(c *gin.Context) {
	c.Set(ctxkey.ToolInvocationCounts, map[string]int{})
	c.Set(ctxkey.WebSearchCallCount, 0)
	c.Set(ctxkey.ToolInvocationSummary, (*model.ToolUsageSummary)(nil))
	c.Set(ctxkey.ClaudeToolAllowanceQuota, int64(0))
}

// admitClaudeMessagesTools applies the shared built-in tool policy to the raw
// Claude Messages body before any reservation or upstream dispatch. The raw body
// is inspected because native passthrough forwards it verbatim. Server tools,
// the MCP connector (mcp_servers / mcp_toolset) and code-execution containers
// must be whitelisted and priced for the selected channel; ordinary custom and
// client-executed tools pass unchanged; unknown typed tools fail closed unless
// the channel explicitly prices that exact type. On success it records the
// conservative paid-tool allowance for the reservation. Parameters: c is the
// request context and meta the mapped request metadata. Returns: a 400 error
// when a capability is not admitted.
func admitClaudeMessagesTools(c *gin.Context, meta *metalib.Meta) *relaymodel.ErrorWithStatusCode {
	resetClaudeToolAttemptState(c)
	raw, err := common.GetRequestBody(c)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "read Claude request for tool admission"), "invalid_claude_messages_request", http.StatusBadRequest)
	}
	uses, unknown, err := collectClaudeServerCapabilities(raw)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_claude_messages_request", http.StatusBadRequest)
	}
	if len(uses) == 0 {
		return nil
	}

	requested := make(map[string]struct{}, len(uses))
	for name := range uses {
		requested[name] = struct{}{}
	}
	provider := relay.GetAdaptor(meta.APIType)
	ctx := gmw.Ctx(c)
	channel := claudeChannelRecord(c)
	quotes, err := tooling.BuiltinToolQuotes(ctx, meta.ActualModelName, meta, channel, provider, requested)
	if err != nil {
		for _, name := range unknown {
			if _, unknownErr := tooling.BuiltinToolQuotes(ctx, meta.ActualModelName, meta, channel, provider, map[string]struct{}{name: {}}); unknownErr != nil {
				err = errors.Wrapf(unknownErr, "unrecognized Claude tool type %q fails closed unless the channel prices it explicitly", name)
				break
			}
		}
		return openai.ErrorWrapper(err, "tool_not_allowed", http.StatusBadRequest)
	}

	var allowance int64
	for name, count := range uses {
		allowance = saturatingQuotaAdd(allowance, tooling.SaturatingToolAllowance(quotes[name], count))
	}
	c.Set(ctxkey.ClaudeToolAllowanceQuota, allowance)
	lg := gmw.GetLogger(c)
	lg.Debug("admitted Claude provider tools",
		zap.Any("tool_uses", uses),
		zap.Int64("tool_allowance_quota", allowance),
	)
	return nil
}

// collectClaudeServerCapabilities classifies every capability requested by a
// raw Claude Messages body. Parameters: raw is the request JSON. Returns: the
// reservation use bound per canonical capability, the unknown tool types, or a
// validation error for malformed tool declarations.
func collectClaudeServerCapabilities(raw []byte) (map[string]int64, []string, error) {
	fields, err := scanExactJSONFields(raw, claudeCapabilityRootFields)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Claude request")
	}
	uses := make(map[string]int64)
	var unknown []string
	if hasJSONValue(fields["tools"]) {
		var rawTools []json.RawMessage
		if err := json.Unmarshal(fields["tools"], &rawTools); err != nil {
			return nil, nil, errors.Wrap(err, "tools must be an array of objects")
		}
		for i, rawTool := range rawTools {
			tool, err := scanExactJSONFields(rawTool, claudeToolPolicyFields)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "tools[%d]", i)
			}
			toolType := ""
			if value, ok := tool["type"]; ok && hasJSONValue(value) {
				if err := json.Unmarshal(value, &toolType); err != nil {
					return nil, nil, errors.Wrapf(err, "tools[%d].type must be a string", i)
				}
			}
			kind, canonical := tooling.ClassifyClaudeTool(toolType)
			switch kind {
			case tooling.ClaudeToolCustom, tooling.ClaudeToolClient:
				continue
			case tooling.ClaudeToolUnknown:
				unknown = append(unknown, canonical)
			}
			uses[canonical] = saturatingQuotaAdd(uses[canonical], claudeToolUseBound(tool["max_uses"]))
		}
	}
	if hasJSONValue(fields["mcp_servers"]) && !bytes.Equal(bytes.TrimSpace(fields["mcp_servers"]), []byte("[]")) {
		uses[tooling.BuiltinMCPConnector] = max(uses[tooling.BuiltinMCPConnector], claudeServerToolDefaultReservedUses)
	}
	if hasJSONValue(fields["container"]) {
		uses[tooling.BuiltinCodeExecution] = max(uses[tooling.BuiltinCodeExecution], claudeServerToolDefaultReservedUses)
	}
	return uses, unknown, nil
}

// scanExactJSONFields token-scans one JSON object and returns the raw values of
// the policy fields under their exact spelling. Passthrough forwards raw bytes,
// so admission must not rely on encoding/json, which matches names
// case-insensitively (including Unicode folds such as U+017F) and keeps the
// last duplicate while other parsers read the exact or first key. Parameters:
// raw is the object JSON and fields the exact policy field names. Returns: the
// values found, or an error when a policy field is duplicated or another key
// folds to it (case-insensitively, ignoring '_' and '-').
func scanExactJSONFields(raw []byte, fields []string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return nil, errors.Wrap(err, "decode JSON object")
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("expected a JSON object")
	}
	values := make(map[string]json.RawMessage, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.Wrap(err, "decode JSON object key")
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid JSON object key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.Wrapf(err, "decode JSON field %q", key)
		}
		field := foldedPolicyField(key, fields)
		if field == "" {
			continue
		}
		if key != field {
			return nil, errors.Errorf("field %q is an ambiguous spelling of %q", key, field)
		}
		if _, duplicate := values[field]; duplicate {
			return nil, errors.Errorf("duplicate field %q", field)
		}
		values[field] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.Wrap(err, "decode JSON object end")
	}
	return values, nil
}

// foldedPolicyField returns the policy field that key would match under
// case-insensitive matching that also ignores '_' and '-', or "" when none does.
func foldedPolicyField(key string, fields []string) string {
	strip := func(r rune) rune {
		if r == '_' || r == '-' {
			return -1
		}
		return r
	}
	folded := strings.Map(strip, key)
	for _, field := range fields {
		if strings.EqualFold(folded, strings.Map(strip, field)) {
			return field
		}
	}
	return ""
}

// claudeToolUseBound returns the invocation bound reserved for one declaration.
// Parameters: raw is the optional max_uses value. Returns: a positive max_uses,
// or the documented default bound when it is absent or not a positive integer.
func claudeToolUseBound(raw json.RawMessage) int64 {
	if !hasJSONValue(raw) {
		return claudeServerToolDefaultReservedUses
	}
	value, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 64)
	if err != nil || value <= 0 {
		return claudeServerToolDefaultReservedUses
	}
	return value
}

// hasJSONValue reports whether raw holds a present, non-null JSON value.
func hasJSONValue(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

// saturatingQuotaAdd adds two non-negative quota amounts without overflowing.
func saturatingQuotaAdd(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// withClaudeToolAllowance adds the admitted paid server-tool allowance to a
// Claude reservation quote. Parameters: c carries the admission and quote is
// the token-based reservation. Returns: the combined reservation quote.
func withClaudeToolAllowance(c *gin.Context, quote int64) int64 {
	return saturatingQuotaAdd(quote, c.GetInt64(ctxkey.ClaudeToolAllowanceQuota))
}

// admitConvertedClaudeBuiltins re-applies the shared Chat or Responses
// validator to a converted upstream request before dispatch, because adaptor
// conversion can introduce built-ins (for example Tool Search mapped to web
// search). Native passthrough is already covered by admitClaudeMessagesTools.
// Parameters: c is the request context, meta the request metadata, converted
// the request the adaptor will send and provider its adaptor. Returns: a 400
// error when the converted request uses a capability the channel disallows.
func admitConvertedClaudeBuiltins(c *gin.Context, meta *metalib.Meta, converted any, provider adaptor.Adaptor) *relaymodel.ErrorWithStatusCode {
	if c.GetBool(ctxkey.ClaudeDirectPassthrough) {
		return nil
	}
	var err error
	switch request := converted.(type) {
	case *relaymodel.GeneralOpenAIRequest:
		err = tooling.ValidateChatBuiltinTools(c, request, meta, claudeChannelRecord(c), provider)
	case *openai.ResponseAPIRequest:
		err = tooling.ValidateResponseBuiltinTools(request, meta, claudeChannelRecord(c), provider)
	}
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "converted Claude request"), "tool_not_allowed", http.StatusBadRequest)
	}
	return nil
}

// applyClaudeServerToolCharges bills the attempt's provider-tool invocations
// exactly once through the shared tooling pricing, merging any MCP summary
// already recorded. A missing usage receipt becomes an explicit estimate so the
// reservation, including the tool allowance, is retained rather than refunded.
// Parameters: c is the request context, usage the settled usage pointer, meta
// the request metadata and provider the dispatching adaptor. Returns: nothing.
func applyClaudeServerToolCharges(c *gin.Context, usage **relaymodel.Usage, meta *metalib.Meta, provider adaptor.Adaptor) {
	if *usage == nil {
		*usage = &relaymodel.Usage{BillingEstimateReason: "missing_usage_retained_reservation"}
	}
	var existing *model.ToolUsageSummary
	if raw, ok := c.Get(ctxkey.ToolInvocationSummary); ok {
		existing, _ = raw.(*model.ToolUsageSummary)
	}
	tooling.ApplyBuiltinToolCharges(c, usage, meta, claudeChannelRecord(c), provider)
	raw, ok := c.Get(ctxkey.ToolInvocationSummary)
	if !ok {
		return
	}
	if current, ok := raw.(*model.ToolUsageSummary); ok && existing != nil && current != nil && current != existing {
		c.Set(ctxkey.ToolInvocationSummary, mergeToolUsageSummaries(existing, current))
	}
}

// claudeChannelRecord returns the selected channel record, or nil when absent.
func claudeChannelRecord(c *gin.Context) *model.Channel {
	if value, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := value.(*model.Channel); ok {
			return channel
		}
	}
	return nil
}
