package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/tooling"
)

// TestScanExactJSONFields verifies only exact, unique policy keys are accepted
// at the scanned level, while escaped exact spellings and nested keys are fine.
func TestScanExactJSONFields(t *testing.T) {
	t.Parallel()
	values, err := scanExactJSONFields([]byte(`{"tools":[{"Tools":1}],"model":"x","container":null,"metadata":{"TOOLS":true}}`), claudeCapabilityRootFields)
	require.NoError(t, err)
	require.JSONEq(t, `[{"Tools":1}]`, string(values["tools"]))
	require.Equal(t, "null", string(values["container"]))
	require.NotContains(t, values, "mcp_servers")
	for _, body := range []string{
		`{"mcp-servers":[]}`, `{"MCP_SERVERS":[]}`, `{"mcpServers":[]}`, `{"Container":"x"}`,
		`{"tools":[],"tools":[]}`, `{"toolſ":[]}`, `[]`, `{"tools":`,
	} {
		_, err := scanExactJSONFields([]byte(body), claudeCapabilityRootFields)
		require.Error(t, err, body)
	}
}

// TestClaudeConservativeIndependentToolAllowance reproduces an underfunded
// Claude request where another paid server capability and a converted DeepSeek
// Search are independently billable. Both must be reserved before dispatch.
func TestClaudeConservativeIndependentToolAllowance(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	meta := &metalib.Meta{
		ChannelType: channeltype.OpenAI, BaseURL: "https://api.deepseek.com",
		ActualModelName: "deepseek-v4-pro",
	}
	// Existing priced server tool: 10 calls x 50 quota, independent of Search.
	c.Set(ctxkey.ClaudeToolAllowanceQuota, int64(500))
	tooling.RecordConservativeDeepSeekTools(c, meta, map[string]int64{
		tooling.BuiltinWebSearch: 17,
	}, nil)
	// The converted Search needs another 10 x 17 rather than reusing the
	// unrelated server tool's reservation.
	require.EqualValues(t, 16+500+170, withClaudeToolAllowance(c, 16))
}

// TestClaudeConservativeRawAndConvertedToolQuoteCompatibility proves that a
// native DeepSeek Search/Fetch declaration is reserved once (even after a
// converted request re-admits it), while unrelated paid server capabilities
// remain independently funded. Other providers preserve their original quote.
func TestClaudeConservativeRawAndConvertedToolQuoteCompatibility(t *testing.T) {
	t.Parallel()
	quotes := map[string]int64{
		tooling.BuiltinWebSearch:     17,
		tooling.BuiltinWebFetch:      23,
		tooling.BuiltinCodeExecution: 50,
	}
	uses := map[string]int64{
		tooling.BuiltinWebSearch:     10,
		tooling.BuiltinWebFetch:      10,
		tooling.BuiltinCodeExecution: 10,
	}
	for _, tc := range []struct {
		name, base string
		raw, total int64
	}{
		{name: "deepseek_native_and_converted_once", base: "https://api.deepseek.com/anthropic", raw: 500, total: 16 + 500 + 170 + 230},
		{name: "other_provider_unchanged", base: "https://api.anthropic.com", raw: 900, total: 16 + 900},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			meta := &metalib.Meta{ChannelType: channeltype.OpenAI, BaseURL: tc.base, ActualModelName: "deepseek-v4-pro"}
			raw := claudeOtherToolAllowance(meta, quotes, uses)
			require.EqualValues(t, tc.raw, raw)
			c.Set(ctxkey.ClaudeToolAllowanceQuota, raw)
			tooling.RecordConservativeDeepSeekTools(c, meta, quotes, uses)
			// A second admission of the same Search must not reserve it twice.
			tooling.RecordConservativeDeepSeekTools(c, meta, map[string]int64{
				tooling.BuiltinWebSearch: 17,
			}, nil)
			require.EqualValues(t, tc.total, withClaudeToolAllowance(c, 16))
		})
	}
}
