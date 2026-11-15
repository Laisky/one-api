package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// storedMCPNumericSchema keeps optional numeric properties so a zero-argument tool call remains valid.
const storedMCPNumericSchema = `{"type":"object","properties":{"count":{"type":"integer","minimum":9007199254740993,"maximum":9007199254740997},"nested":{"type":"object","properties":{"threshold":{"type":"integer","minimum":9007199254740999,"maximum":9007199254741003}}}}}`

// assertStoredMCPNumericSchema checks exact stored numeric constraints in provider and deferred wire carriers.
func assertStoredMCPNumericSchema(t *testing.T, raw json.RawMessage) {
	t.Helper()
	require.NoError(t, validateNumericSchemaTokens(raw, storedMCPNumericTokenChecks()))
}

// TestDeferredMCPStoredNumericSchema preserves stored constraints in actual deferred injection without contacting the tool server.
func TestDeferredMCPStoredNumericSchema(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	server := &model.MCPServer{Name: "exact-deferred-schema-fixture", Status: model.MCPServerStatusEnabled, BaseURL: "https://mcp.fixture.invalid", ToolWhitelist: model.JSONStringSlice{"exact_deferred_probe"}}
	require.NoError(t, model.DB.Create(server).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(server).Error) })
	tool := &model.MCPTool{ServerId: server.Id, Name: "exact_deferred_probe", Status: 1, InputSchema: storedMCPNumericSchema}
	require.NoError(t, model.DB.Create(tool).Error)
	t.Cleanup(func() { require.NoError(t, model.DB.Delete(tool).Error) })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.ChannelModel, &model.Channel{Type: channeltype.Anthropic})
	request := &ClaudeMessagesRequest{Model: "claude-haiku-5-5", Tools: []relaymodel.ClaudeTool{{Type: "tool_search_tool_regex", Name: "tool_search_tool_regex"}}}
	registry, err := injectDeferredMCPToolsForToolSearch(c, request)
	require.NoError(t, err)
	require.NotNil(t, registry)
	require.True(t, registry.isMCPTool(tool.Name))
	var injected *relaymodel.ClaudeTool
	for i := range request.Tools {
		if request.Tools[i].Name == tool.Name {
			injected = &request.Tools[i]
			break
		}
	}
	require.NotNil(t, injected)
	require.True(t, injected.TrustedDeferredLoading)
	require.NotNil(t, injected.DeferLoading)
	require.True(t, *injected.DeferLoading)
	wire, err := json.Marshal(injected.InputSchema)
	require.NoError(t, err)
	assertStoredMCPNumericSchema(t, wire)
	require.Equal(t, storedMCPNumericSchema, tool.InputSchema, "the stored source text remains unchanged")
	args, err := parseToolArguments(`{"count":1.5}`)
	require.NoError(t, err)
	require.IsType(t, float64(0), args["count"], "argument decoding retains its established contract")
}
