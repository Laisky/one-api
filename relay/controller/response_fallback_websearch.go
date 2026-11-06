package controller

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/tooling"
)

// planResponseFallbackClaudeWebSearch decides whether this fallback attempt
// forwards the admitted Responses web_search declaration to a Claude upstream
// as a server-side web search tool. When it does, it records the Claude
// declaration for the request converter and the paid allowance that the Chat
// reservation adds (anthropic.ConvertedWebSearchMaxUses searches at the
// admitted per-call price, the max_uses the converted tool carries). Billing itself uses
// the upstream usage.server_tool_use receipt through the shared server-tool
// path. Platforms without a metered Claude web search keep the historical drop.
//
// Parameters: c is the request context; meta the mapped request metadata;
// channel/provider the admission policy sources; request the hydrated
// Responses request; chatRequest the converted Chat request; mcpActive whether
// MCP orchestration owns this attempt; mcpToolNames the MCP-served builtins.
// Returns: a 400 error for options that cannot be represented, otherwise nil.
func planResponseFallbackClaudeWebSearch(c *gin.Context, meta *metalib.Meta, channel *model.Channel, provider adaptor.Adaptor,
	request *openai.ResponseAPIRequest, chatRequest *relaymodel.GeneralOpenAIRequest, mcpActive bool, mcpToolNames map[string]struct{}) *relaymodel.ErrorWithStatusCode {
	// Every attempt starts without a forwarded search so a retry on another
	// channel never inherits the previous channel's declaration or allowance.
	anthropic.SetConvertedWebSearchTool(c, nil)
	c.Set(ctxkey.ResponseFallbackToolAllowanceQuota, int64(0))

	declaration, ok := responseWebSearchDeclaration(request)
	if !ok {
		return nil
	}
	lg := gmw.GetLogger(c)
	if _, servedByMCP := mcpToolNames[tooling.BuiltinWebSearch]; mcpActive || servedByMCP {
		lg.Debug("response fallback web search stays on the MCP path", zap.Bool("mcp_active", mcpActive), zap.Bool("served_by_mcp", servedByMCP))
		return nil
	}
	toolType := anthropic.ConvertedWebSearchToolType(meta)
	if toolType == "" {
		lg.Debug("response fallback upstream has no metered Claude web search; dropping declaration", zap.Int("channel_type", meta.ChannelType))
		return nil
	}
	if chatRequest != nil {
		for _, tool := range chatRequest.Tools {
			if tool.Function != nil && tool.Function.Name == tooling.BuiltinWebSearch {
				lg.Debug("response fallback web search skipped: a caller tool already uses its name")
				return nil
			}
		}
	}
	converted, err := anthropic.ConvertedWebSearchToolFromResponses(declaration, toolType)
	if err != nil {
		return openai.ErrorWrapper(err, "response_fallback_not_supported", http.StatusBadRequest)
	}
	quotes, err := tooling.BuiltinToolQuotes(gmw.Ctx(c), request.Model, meta, channel, provider, map[string]struct{}{tooling.BuiltinWebSearch: {}})
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "price converted Claude web search"), builtinToolAdmissionErrorCode(err), http.StatusBadRequest)
	}
	allowance := tooling.SaturatingToolAllowance(quotes[tooling.BuiltinWebSearch], int64(anthropic.ConvertedWebSearchMaxUses))
	if deepseekcompat.UsesDeepSeekToolReceiptContract(meta) {
		// The DeepSeek conservative floor recorded at admission already funds
		// Search; reserving it twice would only inflate the hold.
		allowance = 0
	}
	anthropic.SetConvertedWebSearchTool(c, converted)
	c.Set(ctxkey.ResponseFallbackToolAllowanceQuota, allowance)
	lg.Debug("response fallback forwards Claude server web search",
		zap.String("tool_type", toolType),
		zap.Int64("quota_per_search", quotes[tooling.BuiltinWebSearch]),
		zap.Int64("reserved_tool_allowance", allowance),
		zap.Int("allowed_domains", len(converted.AllowedDomains)),
		zap.Int("blocked_domains", len(converted.BlockedDomains)),
		zap.Bool("user_location", converted.UserLocation != nil),
	)
	return nil
}

// responseWebSearchDeclaration returns the first root tool whose canonical
// builtin capability is web search; admission priced exactly these tools.
// Parameters: request is the Responses request. Returns: the declaration and
// whether one exists.
func responseWebSearchDeclaration(request *openai.ResponseAPIRequest) (openai.ResponseAPITool, bool) {
	if request == nil {
		return openai.ResponseAPITool{}, false
	}
	for _, tool := range request.Tools {
		if tooling.NormalizeBuiltinType(tool.Type) == tooling.BuiltinWebSearch {
			return tool, true
		}
	}
	return openai.ResponseAPITool{}, false
}

// responseWebSearchCallID maps a provider search invocation ID onto the ws_
// identifier family Responses clients expect, deterministically so JSON and
// SSE renderings agree. Parameters: upstreamID is the provider ID. Returns: the
// Responses item ID; an empty upstream ID gets a random one.
func responseWebSearchCallID(upstreamID string) string {
	trimmed := strings.TrimSpace(upstreamID)
	if trimmed == "" {
		return "ws_" + random.GetRandomString(24)
	}
	if strings.HasPrefix(trimmed, "ws_") {
		return trimmed
	}
	return "ws_" + strings.TrimPrefix(trimmed, "srvtoolu_")
}

// responseWebSearchCallItem renders one web_search_call output item.
// Parameters: id is the Responses item ID, status its status and query the
// searched text. Returns: the item with a search action.
func responseWebSearchCallItem(id, status, query string) openai.OutputItem {
	return openai.OutputItem{
		Type:   "web_search_call",
		Id:     id,
		Status: status,
		Action: &openai.WebSearchCallAction{Type: "search", Query: query},
	}
}

// finalWebSearchCallStatus maps a carried status onto a terminal Responses
// status: anything that did not complete is reported as failed.
func finalWebSearchCallStatus(status string) string {
	if status == relaymodel.WebSearchCallCompleted {
		return relaymodel.WebSearchCallCompleted
	}
	return relaymodel.WebSearchCallFailed
}

// webSearchCallOutputItems renders the completed web searches of one
// non-streaming converted message. Parameters: calls are the carried searches.
// Returns: the web_search_call items in upstream order.
func webSearchCallOutputItems(calls []relaymodel.WebSearchCall) []openai.OutputItem {
	if len(calls) == 0 {
		return nil
	}
	items := make([]openai.OutputItem, 0, len(calls))
	for _, call := range calls {
		items = append(items, responseWebSearchCallItem(responseWebSearchCallID(call.ID), finalWebSearchCallStatus(call.Status), call.Query))
	}
	return items
}
