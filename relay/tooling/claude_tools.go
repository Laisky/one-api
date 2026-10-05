package tooling

import (
	"context"
	"math"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// Canonical built-in capability names shared by Chat, Responses and Claude
// Messages policy, pricing and invocation counters.
const (
	// BuiltinWebSearch is the canonical paid web search capability.
	BuiltinWebSearch = "web_search"
	// BuiltinWebFetch is Anthropic's server-side URL fetch capability.
	BuiltinWebFetch = "web_fetch"
	// BuiltinCodeExecution is a provider-hosted code execution sandbox.
	BuiltinCodeExecution = "code_execution"
	// BuiltinToolSearch is Anthropic's deferred tool-definition search. It is a
	// distinct, unmetered capability and must never be billed as web search.
	BuiltinToolSearch = "tool_search"
	// BuiltinMCPConnector is provider-side remote MCP execution (mcp_servers / mcp_toolset).
	BuiltinMCPConnector = "mcp_connector"
)

// zeroCostBuiltins lists capabilities the provider documents as never metered.
// They are admitted at a zero tariff unless a channel whitelist excludes them or
// a channel explicitly prices them.
var zeroCostBuiltins = []string{BuiltinToolSearch}

// anthropicDatedServerTool matches Anthropic's versioned server-tool type names
// (for example web_search_20260209). Only dated forms are recognized so that
// other providers' suffixed names (web_search_preview_reasoning) keep their
// own pricing keys.
var anthropicDatedServerTool = regexp.MustCompile(`^(web_search|web_fetch|code_execution)_\d{8}$`)

// anthropicClientTool matches Anthropic tool types that are executed by the
// caller's own client. The provider only emits tool_use blocks for them.
var anthropicClientTool = regexp.MustCompile(`^(bash|text_editor|computer|computer_toolset|browser_toolset|memory)_\d{8}$`)

// ClaudeToolKind classifies one Claude Messages tool declaration.
type ClaudeToolKind int

const (
	// ClaudeToolCustom is an ordinary caller-defined tool (no type, "custom" or "function").
	ClaudeToolCustom ClaudeToolKind = iota
	// ClaudeToolClient is an Anthropic-defined tool executed by the caller's client.
	ClaudeToolClient
	// ClaudeToolServer is a provider-executed capability subject to channel policy.
	ClaudeToolServer
	// ClaudeToolUnknown is a typed tool the gateway cannot classify.
	ClaudeToolUnknown
)

// ClassifyClaudeTool maps a Claude Messages tool type to its kind and, for
// provider-executed tools, its canonical capability name. Parameters: toolType
// is the raw "type" value. Returns: the kind and the canonical policy key; the
// key is the lower-cased raw type for unknown tools and empty for caller tools.
//
// Unknown typed tools are classified separately so callers fail closed: a new
// provider tool may carry a charge or remote side effects the gateway cannot
// meter, so it is rejected unless an operator explicitly prices that exact type.
func ClassifyClaudeTool(toolType string) (ClaudeToolKind, string) {
	normalized := strings.ToLower(strings.TrimSpace(toolType))
	switch normalized {
	case "", "custom", "function":
		return ClaudeToolCustom, ""
	case BuiltinWebFetch, BuiltinCodeExecution:
		return ClaudeToolServer, normalized
	case "mcp_toolset":
		return ClaudeToolServer, BuiltinMCPConnector
	}
	if canonical := NormalizeBuiltinType(normalized); canonical != "" {
		return ClaudeToolServer, canonical
	}
	if anthropicClientTool.MatchString(normalized) {
		return ClaudeToolClient, ""
	}
	return ClaudeToolUnknown, normalized
}

// BuiltinToolQuotes validates requested canonical built-in tools against the
// shared channel/provider policy and returns their per-call quota. Parameters:
// ctx carries request correlation, modelName/meta identify the effective model,
// channel/provider supply policy, and requested holds canonical names. Returns:
// per-call quota for every requested tool, or an error naming the first tool
// that is not whitelisted or has no tariff.
func BuiltinToolQuotes(ctx context.Context, modelName string, meta *metalib.Meta, channel *model.Channel, provider adaptor.Adaptor, requested map[string]struct{}) (map[string]int64, error) {
	quotes := make(map[string]int64, len(requested))
	if len(requested) == 0 {
		return quotes, nil
	}
	effectiveModel := resolveModelName(meta, modelName)
	policy := buildToolPolicyWithContext(ctx, channel, effectiveToolProvider(meta, channel, provider), effectiveModel)
	for toolName := range requested {
		canonical := strings.ToLower(strings.TrimSpace(toolName))
		if !policy.isAllowed(canonical) {
			return nil, errors.Errorf("tool %s is not allowed on this channel (model=%s); update the tooling whitelist or pricing", canonical, effectiveModel)
		}
		quotes[canonical] = policy.pricing[canonical]
	}
	return quotes, nil
}

// effectiveToolProvider drops provider defaults for Azure channels, whose
// built-in tools are governed only by explicit channel configuration.
// Parameters: meta/channel identify the channel type and provider is the
// adaptor. Returns: the provider whose defaults participate in policy.
func effectiveToolProvider(meta *metalib.Meta, channel *model.Channel, provider adaptor.Adaptor) adaptor.Adaptor {
	if channel != nil {
		if channel.Type == channeltype.Azure {
			return nil
		}
		return provider
	}
	if meta != nil && meta.ChannelType == channeltype.Azure {
		return nil
	}
	return provider
}

// SaturatingToolAllowance multiplies a per-call quota by a use bound without
// overflowing. Parameters: perCall is quota per invocation and uses the bound.
// Returns: the product, saturated at math.MaxInt64, or zero for non-positive input.
func SaturatingToolAllowance(perCall int64, uses int64) int64 {
	if perCall <= 0 || uses <= 0 {
		return 0
	}
	if perCall > math.MaxInt64/uses {
		return math.MaxInt64
	}
	return perCall * uses
}
