package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// marshalIDForFixture serializes a JSON-RPC id field back to its raw JSON
// form so the fake upstream can echo it correctly across number/string
// types. Returns the literal `null` if the id is missing or unmarshalable.
func marshalIDForFixture(id any) string {
	if id == nil {
		return "null"
	}
	encoded, err := json.Marshal(id)
	if err != nil {
		return "null"
	}
	return string(encoded)
}

// setupMCPProxyTest spins up an isolated SQLite database, fake MCP backend,
// and a test user/server/tool seeded for callMCPToolForUser-driven scenarios.
// The returned mcpFixture lets tests configure pricing, swap the upstream
// response, and inspect logged invocations.
type mcpFixture struct {
	user           *model.User
	server         *model.MCPServer
	tool           *model.MCPTool
	upstream       *httptest.Server
	upstreamHits   int
	respondPayload func() ([]byte, int)
}

func setupMCPProxyTest(t *testing.T) (cleanup func(), fx *mcpFixture) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:mcp_proxy_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.MCPServer{},
		&model.MCPTool{},
		&model.Log{},
	))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	originalDB := model.DB
	originalLOG := model.LOG_DB
	model.DB = db
	model.LOG_DB = db

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)

	originalRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)

	originalTimeout := config.MCPToolCallTimeoutSec
	config.MCPToolCallTimeoutSec = 5

	fx = &mcpFixture{}

	// Default upstream returns a successful CallToolResult; tests can override.
	fx.respondPayload = func() ([]byte, int) {
		return []byte(`{"jsonrpc":"2.0","id":"1","result":{"content":"ok","is_error":false}}`), http.StatusOK
	}

	// The fake upstream behaves like a spec-compliant MCP server: it replies
	// to `initialize` with a valid handshake envelope, accepts
	// notifications with HTTP 202, and routes tool calls to the
	// configurable respondPayload. upstreamHits counts only tool calls so
	// existing assertions still mean "the proxy invoked the tool once."
	fx.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var rpc struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.Unmarshal(bodyBytes, &rpc)

		switch rpc.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}},"serverInfo":{"name":"fake-upstream","version":"1.0"}}}`, marshalIDForFixture(rpc.ID))
			return
		}
		if strings.HasPrefix(rpc.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		fx.upstreamHits++
		body, status := fx.respondPayload()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))

	user := &model.User{
		Id:       42,
		Username: "mcp-proxy-user",
		Password: "password",
		Role:     model.RoleCommonUser,
		Status:   model.UserStatusEnabled,
		Quota:    1000,
	}
	require.NoError(t, model.DB.Create(user).Error)
	fx.user = user

	server := &model.MCPServer{
		Id:                      1,
		Name:                    "fake-mcp",
		Status:                  model.MCPServerStatusEnabled,
		BaseURL:                 fx.upstream.URL,
		Protocol:                model.MCPProtocolStreamableHTTP,
		AuthType:                model.MCPAuthTypeNone,
		ToolWhitelist:           model.JSONStringSlice{"echo", "paid_echo"},
		AutoSyncIntervalMinutes: 60,
	}
	require.NoError(t, model.DB.Create(server).Error)
	// Re-load via store helper to mirror runtime decryption path; ensures the
	// fixture-configured pricing survives the round trip used in production.
	loaded, err := model.GetMCPServerByName(server.Name)
	require.NoError(t, err)
	fx.server = loaded

	tool := &model.MCPTool{
		Id:          1,
		ServerId:    server.Id,
		Name:        "echo",
		DisplayName: "Echo",
		Description: "Echoes input",
		InputSchema: `{"type":"object"}`,
		Status:      1,
	}
	require.NoError(t, model.DB.Create(tool).Error)
	fx.tool = tool

	cleanup = func() {
		fx.upstream.Close()
		model.DB = originalDB
		model.LOG_DB = originalLOG
		common.UsingSQLite.Store(originalUsingSQLite)
		common.SetRedisEnabled(originalRedis)
		config.MCPToolCallTimeoutSec = originalTimeout
	}

	return cleanup, fx
}

// setToolPricing updates the persisted MCP server's per-tool pricing map and
// reloads the fixture's cached server pointer so subsequent calls observe it.
func (fx *mcpFixture) setToolPricing(t *testing.T, toolName string, pricing model.ToolPricingLocal) {
	t.Helper()
	pricingMap := model.MCPToolPricingMap{toolName: pricing}
	require.NoError(t, model.DB.Model(&model.MCPServer{}).
		Where("id = ?", fx.server.Id).
		Update("tool_pricing", pricingMap).Error)
	loaded, err := model.GetMCPServerByName(fx.server.Name)
	require.NoError(t, err)
	fx.server = loaded
}

// newMCPCallContext builds a gin context wired with the user/request id keys
// the MCP proxy reads, plus a logger so structured logging does not panic.
func newMCPCallContext(t *testing.T, userID int, requestID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(nil))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	c.Set(ctxkey.Id, userID)
	c.Set(ctxkey.RequestId, requestID)
	c.Set(helper.RequestIdKey, requestID)
	gmw.SetLogger(c, logger.Logger)

	return c, recorder
}

// TestCallMCPToolForUser_FreeToolEmitsLog verifies that a zero-priced tool
// invocation still produces a LogTypeTool row so free MCP calls share the
// same audit trail as paid ones.
func TestCallMCPToolForUser_FreeToolEmitsLog(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	c, _ := newMCPCallContext(t, fx.user.Id, "req-free")
	result, err := callMCPToolForUser(context.Background(), c, mcpCallParams{
		Name:      "fake-mcp.echo",
		Arguments: map[string]any{"input": "hello"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)
	require.Equal(t, 1, fx.upstreamHits)

	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-free").Find(&rows).Error)
	require.Len(t, rows, 1, "free tool call must emit exactly one log row")

	row := rows[0]
	require.Equal(t, model.LogTypeTool, row.Type)
	require.Equal(t, "fake-mcp.echo", row.ModelName)
	require.Equal(t, 0, row.Quota, "free invocations must record zero quota")
	require.Equal(t, fx.user.Id, row.UserId)
	require.False(t, row.IsStream)
	require.Contains(t, row.Content, "MCP tool call: fake-mcp.echo")
	require.GreaterOrEqual(t, row.ElapsedTime, int64(1),
		"elapsed time must reflect the actual call duration, not the prior placeholder")

	// User quota must remain untouched on a free call.
	refreshed, err := model.GetUserById(fx.user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(1000), refreshed.Quota)
	require.Equal(t, int64(0), refreshed.UsedQuota)
	require.Equal(t, 0, refreshed.RequestCount)
}

// TestCallMCPToolForUser_PaidToolDeductsQuotaAndLogs ensures the paid path
// continues to deduct quota and that the log row carries the correct cost.
func TestCallMCPToolForUser_PaidToolDeductsQuotaAndLogs(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	fx.setToolPricing(t, "echo", model.ToolPricingLocal{QuotaPerCall: 75})

	c, _ := newMCPCallContext(t, fx.user.Id, "req-paid")
	result, err := callMCPToolForUser(context.Background(), c, mcpCallParams{
		Name:      "fake-mcp.echo",
		Arguments: map[string]any{"input": "paid"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, fx.upstreamHits)

	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-paid").Find(&rows).Error)
	require.Len(t, rows, 1)

	row := rows[0]
	require.Equal(t, model.LogTypeTool, row.Type)
	require.Equal(t, "fake-mcp.echo", row.ModelName)
	require.Equal(t, 75, row.Quota)

	refreshed, err := model.GetUserById(fx.user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(925), refreshed.Quota)
	require.Equal(t, int64(75), refreshed.UsedQuota)
	require.Equal(t, 1, refreshed.RequestCount)
}

// TestCallMCPToolForUser_ErrorResultIsNotLoggedOrCharged confirms that when
// the MCP backend returns an is_error result we neither bill nor log,
// preserving the existing contract for tool-side failures.
func TestCallMCPToolForUser_ErrorResultIsNotLoggedOrCharged(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	fx.setToolPricing(t, "echo", model.ToolPricingLocal{QuotaPerCall: 50})
	fx.respondPayload = func() ([]byte, int) {
		return []byte(`{"jsonrpc":"2.0","id":"1","result":{"content":"boom","is_error":true}}`), http.StatusOK
	}

	c, _ := newMCPCallContext(t, fx.user.Id, "req-err")
	result, err := callMCPToolForUser(context.Background(), c, mcpCallParams{
		Name: "fake-mcp.echo",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.IsError)

	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-err").Find(&rows).Error)
	require.Empty(t, rows, "tool-error results must not produce billing logs")

	refreshed, err := model.GetUserById(fx.user.Id, true)
	require.NoError(t, err)
	require.Equal(t, int64(1000), refreshed.Quota,
		"tool-error must not deduct user quota")
}

// TestCallMCPToolForUser_TransportFailureProducesNoLog covers the case where
// the upstream MCP server returns a non-2xx HTTP status. The proxy should
// surface the error and not persist a partial billing row.
func TestCallMCPToolForUser_TransportFailureProducesNoLog(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	fx.respondPayload = func() ([]byte, int) {
		return []byte(`{"error":"upstream down"}`), http.StatusInternalServerError
	}

	c, _ := newMCPCallContext(t, fx.user.Id, "req-transport")
	_, err := callMCPToolForUser(context.Background(), c, mcpCallParams{
		Name: "fake-mcp.echo",
	})
	require.Error(t, err)

	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-transport").Find(&rows).Error)
	require.Empty(t, rows, "transport failures must not produce billing logs")
}

// TestMCPProxy_Initialize verifies the Streamable HTTP handshake: an
// `initialize` request must return protocolVersion, capabilities, and
// serverInfo so MCP Inspector / SDK clients accept the connection.
func TestMCPProxy_Initialize(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	body := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"mcp-inspector","version":"0.21"}}}`

	c, recorder := newMCPCallContext(t, fx.user.Id, "req-init")
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Accept", "application/json, text/event-stream")

	MCPProxy(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, "2.0", resp["jsonrpc"])
	require.Nil(t, resp["error"])

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "result must be an object")
	require.NotEmpty(t, result["protocolVersion"], "protocolVersion required by spec")

	caps, ok := result["capabilities"].(map[string]any)
	require.True(t, ok, "capabilities required by spec")
	require.Contains(t, caps, "tools", "server must advertise tools capability")

	info, ok := result["serverInfo"].(map[string]any)
	require.True(t, ok, "serverInfo required by spec")
	require.NotEmpty(t, info["name"])
	require.NotEmpty(t, info["version"])
}

// TestMCPProxy_NotificationsInitialized confirms the server replies to the
// `notifications/initialized` notification with HTTP 202 and an empty body —
// returning a JSON-RPC envelope here breaks SDK clients.
func TestMCPProxy_NotificationsInitialized(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`

	c, recorder := newMCPCallContext(t, fx.user.Id, "req-notif")
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	MCPProxy(c)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.Empty(t, recorder.Body.Bytes(), "notifications must not produce a JSON-RPC response body")
}

// TestMCPProxy_Ping verifies the `ping` request returns an empty result.
func TestMCPProxy_Ping(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	body := `{"jsonrpc":"2.0","id":"p1","method":"ping"}`

	c, recorder := newMCPCallContext(t, fx.user.Id, "req-ping")
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	MCPProxy(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, "p1", resp["id"])
	require.Nil(t, resp["error"])
	require.NotNil(t, resp["result"])
}

// TestMCPProxy_MethodNotFound verifies that an unknown request method returns
// a JSON-RPC error with code -32601 (method not found) per spec.
func TestMCPProxy_MethodNotFound(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	body := `{"jsonrpc":"2.0","id":9,"method":"completely/unknown"}`

	c, recorder := newMCPCallContext(t, fx.user.Id, "req-unknown")
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	MCPProxy(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok, "expected JSON-RPC error envelope")
	require.Equal(t, float64(mcpErrMethodNotFound), errObj["code"])
}

// TestMCPProxy_GetReturns405 ensures the stateless proxy rejects GET requests
// (no server-initiated streaming) with 405 Method Not Allowed, as required
// by the Streamable HTTP transport spec.
func TestMCPProxy_GetReturns405(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	c, recorder := newMCPCallContext(t, fx.user.Id, "req-get")
	c.Request = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	c.Request.Header.Set("Accept", "text/event-stream")

	MCPProxy(c)
	require.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
}

// TestMCPProxy_SelfTest_ClientDrivesOwnServer wires the in-process
// StreamableHTTPClient against our own MCPProxy handler over real HTTP.
// This is the bidirectional contract: the same protocol code that talks to
// upstream MCP servers must also be able to drive our own server. If
// either half drifts from the spec, this test fails first.
func TestMCPProxy_SelfTest_ClientDrivesOwnServer(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// Stub the auth middleware: inject the fixture user into context.
	// Production uses TokenAuth; here we bypass it because the unit under
	// test is the protocol contract, not authentication.
	router.Any("/mcp", func(c *gin.Context) {
		c.Set(ctxkey.Id, fx.user.Id)
		c.Set(ctxkey.RequestId, "selftest")
		c.Set(helper.RequestIdKey, "selftest")
		gmw.SetLogger(c, logger.Logger)
		MCPProxy(c)
	})
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()

	// Build a real client pointing at our own /mcp endpoint and exercise
	// the full lifecycle: initialize → notifications/initialized →
	// tools/list. The fixture seeds a tool named "fake-mcp.echo".
	mcpServer := &model.MCPServer{
		BaseURL:  httpServer.URL + "/mcp",
		AuthType: model.MCPAuthTypeNone,
	}
	client := mcp.NewStreamableHTTPClient(mcpServer, nil, 5*time.Second)

	require.NoError(t, client.Initialize(context.Background()),
		"initialize handshake against our own server must succeed")

	tools, err := client.ListTools(context.Background())
	require.NoError(t, err, "tools/list against our own server must succeed")

	var names []string
	for _, tl := range tools {
		names = append(names, tl.Name)
	}
	require.Contains(t, names, "fake-mcp.echo",
		"client must surface the seeded tool via the proxy: got %v", names)
}

// TestMCPProxy_HTTPHandlerLogsFreeToolCall exercises the full HTTP entry
// point so that the JSON-RPC envelope, dispatch, and unified logging behave
// end-to-end for a free tool invocation.
func TestMCPProxy_HTTPHandlerLogsFreeToolCall(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()

	body := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"fake-mcp.echo","arguments":{"input":"end-to-end"}}}`

	c, recorder := newMCPCallContext(t, fx.user.Id, "req-handler")
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	MCPProxy(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, "2.0", resp["jsonrpc"])
	require.Nil(t, resp["error"], "successful call must not carry an error envelope: %v", resp["error"])
	require.NotNil(t, resp["result"])

	var rows []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-handler").Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, model.LogTypeTool, rows[0].Type)
	require.Equal(t, 0, rows[0].Quota)
	require.Equal(t, "fake-mcp.echo", rows[0].ModelName)
}
