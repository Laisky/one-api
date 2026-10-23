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
