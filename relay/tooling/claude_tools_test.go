package tooling

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	awsadaptor "github.com/Laisky/one-api/relay/adaptor/aws"
	deepseekadaptor "github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestClassifyClaudeTool verifies the shared Claude tool classification used
// by native and converted Claude Messages admission.
func TestClassifyClaudeTool(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		toolType  string
		kind      ClaudeToolKind
		canonical string
	}{
		{"", ClaudeToolCustom, ""},
		{"custom", ClaudeToolCustom, ""},
		{"function", ClaudeToolCustom, ""},
		{"web_search_20250305", ClaudeToolServer, BuiltinWebSearch},
		{"web_search_20260318", ClaudeToolServer, BuiltinWebSearch},
		{"WEB_SEARCH_20260209", ClaudeToolServer, BuiltinWebSearch},
		{"web_fetch_20250910", ClaudeToolServer, BuiltinWebFetch},
		{"web_fetch", ClaudeToolServer, BuiltinWebFetch},
		{"code_execution_20250825", ClaudeToolServer, BuiltinCodeExecution},
		{"code_execution", ClaudeToolServer, BuiltinCodeExecution},
		{"tool_search_tool_regex_20251119", ClaudeToolServer, BuiltinToolSearch},
		{"tool_search_tool_bm25", ClaudeToolServer, BuiltinToolSearch},
		{"mcp_toolset", ClaudeToolServer, BuiltinMCPConnector},
		{"bash_20250124", ClaudeToolClient, ""},
		{"text_editor_20250728", ClaudeToolClient, ""},
		{"computer_20251124", ClaudeToolClient, ""},
		{"computer_toolset_20260801", ClaudeToolClient, ""},
		{"browser_toolset_20260801", ClaudeToolClient, ""},
		{"memory_20250818", ClaudeToolClient, ""},
		{"advisor_20260301", ClaudeToolUnknown, "advisor_20260301"},
		{"bash", ClaudeToolUnknown, "bash"},
	} {
		kind, canonical := ClassifyClaudeTool(tc.toolType)
		require.Equal(t, tc.kind, kind, tc.toolType)
		require.Equal(t, tc.canonical, canonical, tc.toolType)
	}
}

// TestCanonicalNamesMatchAnthropicReceipts keeps receipt counters and policy keys aligned.
func TestCanonicalNamesMatchAnthropicReceipts(t *testing.T) {
	t.Parallel()
	require.Equal(t, BuiltinWebSearch, anthropic.ToolTypeWebSearch)
	require.Equal(t, BuiltinWebFetch, anthropic.ToolTypeWebFetch)
	require.Equal(t, BuiltinCodeExecution, anthropic.ToolTypeCodeExecution)
	require.Equal(t, BuiltinToolSearch, anthropic.ToolTypeToolSearch)
	require.Equal(t, BuiltinMCPConnector, anthropic.ToolTypeMCPConnector)
}

// TestBuiltinToolQuotesProviderDefaults verifies the published server-tool
// tariffs and fail-closed defaults for every Claude-serving provider family.
func TestBuiltinToolQuotesProviderDefaults(t *testing.T) {
	t.Parallel()
	searchQuota := int64(math.Ceil(0.01 * float64(ratio.QuotaPerUsd)))
	for _, tc := range []struct {
		name     string
		provider adaptor.Adaptor
		model    string
		allowed  map[string]int64
		rejected []string
	}{
		{name: "anthropic", provider: &anthropic.Adaptor{}, model: "claude-sonnet-4",
			allowed:  map[string]int64{BuiltinWebSearch: searchQuota, BuiltinWebFetch: 0, BuiltinToolSearch: 0},
			rejected: []string{BuiltinCodeExecution, BuiltinMCPConnector, "advisor_20260301"}},
		{name: "bedrock", provider: &awsadaptor.Adaptor{}, model: "claude-sonnet-4-5",
			allowed:  map[string]int64{BuiltinToolSearch: 0},
			rejected: []string{BuiltinWebSearch, BuiltinWebFetch, BuiltinCodeExecution, BuiltinMCPConnector}},
		{name: "vertex_claude", provider: &vertexai.Adaptor{}, model: "claude-sonnet-4@20250514",
			allowed:  map[string]int64{BuiltinWebSearch: searchQuota, BuiltinToolSearch: 0},
			rejected: []string{BuiltinWebFetch, BuiltinCodeExecution, BuiltinMCPConnector}},
		{name: "vertex_gemini_unchanged", provider: &vertexai.Adaptor{}, model: "gemini-2.5-pro",
			allowed:  map[string]int64{BuiltinToolSearch: 0},
			rejected: []string{BuiltinWebSearch}},
		{name: "openai_converted", provider: &openai.Adaptor{}, model: "gpt-4o",
			allowed:  map[string]int64{BuiltinWebSearch: searchQuota, BuiltinToolSearch: 0},
			rejected: []string{BuiltinWebFetch, BuiltinCodeExecution}},
		{name: "deepseek_no_builtins", provider: &deepseekadaptor.Adaptor{}, model: "deepseek-chat",
			allowed:  map[string]int64{BuiltinToolSearch: 0},
			rejected: []string{BuiltinWebSearch}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			meta := &metalib.Meta{ActualModelName: tc.model}
			for name, want := range tc.allowed {
				quotes, err := BuiltinToolQuotes(context.Background(), tc.model, meta, nil, tc.provider, map[string]struct{}{name: {}})
				require.NoError(t, err, name)
				require.Equal(t, want, quotes[name], name)
			}
			for _, name := range tc.rejected {
				_, err := BuiltinToolQuotes(context.Background(), tc.model, meta, nil, tc.provider, map[string]struct{}{name: {}})
				require.ErrorContains(t, err, "not allowed", name)
			}
		})
	}
}

// TestBuiltinToolQuotesChannelPolicy verifies channel whitelists can exclude
// free capabilities, channel pricing can opt into fail-closed capabilities, and
// Azure ignores provider defaults.
func TestBuiltinToolQuotesChannelPolicy(t *testing.T) {
	t.Parallel()
	meta := &metalib.Meta{ActualModelName: "claude-sonnet-4"}
	whitelisted := &model.Channel{Type: channeltype.Anthropic}
	require.NoError(t, whitelisted.SetToolingConfig(&model.ChannelToolingConfig{Whitelist: []string{"web_search"}}))
	_, err := BuiltinToolQuotes(context.Background(), "", meta, whitelisted, &anthropic.Adaptor{}, map[string]struct{}{BuiltinToolSearch: {}})
	require.ErrorContains(t, err, "tool_search")

	optIn := &model.Channel{Type: channeltype.Anthropic}
	require.NoError(t, optIn.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{
		BuiltinCodeExecution: {QuotaPerCall: 7},
		BuiltinToolSearch:    {QuotaPerCall: 3},
		BuiltinMCPConnector:  {},
	}}))
	quotes, err := BuiltinToolQuotes(context.Background(), "", meta, optIn, &anthropic.Adaptor{}, map[string]struct{}{BuiltinCodeExecution: {}, BuiltinToolSearch: {}, BuiltinMCPConnector: {}})
	require.NoError(t, err)
	require.Equal(t, map[string]int64{BuiltinCodeExecution: 7, BuiltinToolSearch: 3, BuiltinMCPConnector: 0}, quotes)

	azure := &model.Channel{Type: channeltype.Azure}
	_, err = BuiltinToolQuotes(context.Background(), "", meta, azure, &anthropic.Adaptor{}, map[string]struct{}{BuiltinWebSearch: {}})
	require.ErrorContains(t, err, "web_search", "Azure channels require explicit tool pricing")
}

// TestSaturatingToolAllowance verifies the reservation product never overflows.
func TestSaturatingToolAllowance(t *testing.T) {
	t.Parallel()
	require.Equal(t, int64(51), SaturatingToolAllowance(17, 3))
	require.Zero(t, SaturatingToolAllowance(0, 3))
	require.Zero(t, SaturatingToolAllowance(17, 0))
	require.Equal(t, int64(math.MaxInt64), SaturatingToolAllowance(math.MaxInt64/2, 3))
}
