package anthropic

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestDeepSeekToolReceiptClaude prevents an Anthropic-compatible shape from
// turning undocumented DeepSeek counters or fallback blocks into paid receipts.
func TestDeepSeekToolReceiptClaude(t *testing.T) {
	for _, tc := range []struct {
		name, base    string
		channel, want int
	}{
		{"dedicated", "https://fixture.invalid", channeltype.DeepSeek, 0},
		{"official_compatible_host", "https://api.deepseek.com/anthropic", channeltype.Anthropic, 0},
		{"unrelated_anthropic", "https://api.anthropic.com", channeltype.Anthropic, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.Channel, tc.channel)
			c.Set(ctxkey.BaseURL, tc.base)
			c.Set(ctxkey.ToolInvocationCounts, map[string]int{"file_search": 3})
			for _, body := range []string{
				`{"content":[{"type":"server_tool_use","name":"web_search","input":{}},{"type":"server_tool_use","name":"web_fetch","input":{}},{"type":"tool_use","name":"web_search","input":{}}],"usage":{}}`,
				`{"content":[],"usage":{"server_tool_use":{"web_search_requests":1,"web_fetch_requests":1}}}`,
			} {
				require.NoError(t, RecordServerToolUseFromJSON(c, []byte(body)))
			}
			raw, exists := c.Get(ctxkey.ToolInvocationCounts)
			require.True(t, exists)
			counts := raw.(map[string]int)
			require.Equal(t, 3, counts["file_search"], "unrelated counters are untouched")
			require.Equal(t, tc.want, counts["web_search"])
			require.Equal(t, tc.want, counts["web_fetch"])
		})
	}
}

// TestDeepSeekToolReceiptEndpointClaudeCounters checks both native tool names
// against the selected endpoint while ignoring the prior attempt's observed URL.
func TestDeepSeekToolReceiptEndpointClaudeCounters(t *testing.T) {
	for _, tc := range []struct {
		name, base, override, observed string
		want                           int
	}{
		{"planned_deepseek", "https://api.anthropic.com", "https://api.deepseek.com/anthropic/v1/messages", "https://api.anthropic.com/v1/messages", 0},
		{"reverse", "https://api.deepseek.com", "https://api.anthropic.com/v1/messages", "https://api.deepseek.com/anthropic/v1/messages", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			mode := relaymode.GetByPath("/v1/messages")
			metalib.Set2Context(c, &metalib.Meta{ChannelType: channeltype.Anthropic, BaseURL: tc.base, Mode: mode, UpstreamRequestURL: tc.observed, Config: model.ChannelConfig{EndpointURLs: map[string]string{channeltype.RelayModeToEndpointName(mode): tc.override}}})
			require.NoError(t, RecordServerToolUseFromJSON(c, []byte(`{"content":[],"usage":{"server_tool_use":{"web_search_requests":1,"web_fetch_requests":1}}}`)))
			raw, _ := c.Get(ctxkey.ToolInvocationCounts)
			counts, _ := raw.(map[string]int)
			require.Equal(t, tc.want, counts["web_search"])
			require.Equal(t, tc.want, counts["web_fetch"])
		})
	}
}

// TestDeepSeekReceiptAliasesPreserveUnrelatedProviderCounts validates that
// contract suppression normalizes capability identity without changing the
// existing raw multi-key counter merge for an unrelated Anthropic host.
func TestDeepSeekReceiptAliasesPreserveUnrelatedProviderCounts(t *testing.T) {
	for _, tc := range []struct {
		name, base string
		want       int
	}{
		{"deepseek", "https://api.deepseek.com/anthropic", 0},
		{"unrelated", "https://api.anthropic.com", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.Channel, channeltype.Anthropic)
			c.Set(ctxkey.BaseURL, tc.base)
			require.NoError(t, RecordServerToolUseFromJSON(c, []byte(`{"content":[],"usage":{"server_tool_use":{"web_search_requests":1,"web_search_preview_requests":1}}}`)))
			raw, _ := c.Get(ctxkey.ToolInvocationCounts)
			counts, _ := raw.(map[string]int)
			if tc.want == 0 {
				require.Empty(t, counts)
			} else {
				require.Equal(t, 1, counts["web_search"])
				require.Equal(t, 1, counts["web_search_preview"])
			}
		})
	}
}
