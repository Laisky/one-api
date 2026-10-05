package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
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
