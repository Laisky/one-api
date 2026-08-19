package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

type uuidContractFixture struct {
	user       *model.User
	token      *model.Token
	channel    *model.Channel
	redemption *model.Redemption
	mcpServer  *model.MCPServer
	mcpTool    *model.MCPTool
	trace      *model.Trace
	log        *model.Log
	cost       *model.UserRequestCost
	passkey    *model.PasskeyCredential
}

// setupUUIDContractTestEnvironment creates an isolated database and seeds representative UUID-backed rows.
// Parameters:
//   - t: active test handle.
//
// Return values:
//   - *uuidContractFixture: seeded rows used by contract assertions.
//   - func(): cleanup function that restores global model/database state.
func setupUUIDContractTestEnvironment(t *testing.T) (*uuidContractFixture, func()) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Channel{},
		&model.Token{},
		&model.Redemption{},
		&model.MCPServer{},
		&model.MCPTool{},
		&model.Trace{},
		&model.Log{},
		&model.UserRequestCost{},
		&model.PasskeyCredential{},
		&model.Ability{},
	))

	originalDB := model.DB
	originalLOGDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite.Load()
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite.Store(true)

	user := &model.User{
		Username:    "uuid-contract-user",
		Password:    "hashed",
		DisplayName: "UUID Contract User",
		Group:       "default",
		Status:      model.UserStatusEnabled,
		Role:        model.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(user).Error)
	require.NotEmpty(t, user.UUID)

	channel := &model.Channel{
		Type:   1,
		Name:   "uuid-contract-channel",
		Models: "gpt-4o",
		Config: "{}",
		Status: model.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NotEmpty(t, channel.UUID)

	token := &model.Token{
		UserId:       user.Id,
		UserUUID:     &user.UUID,
		Key:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Name:         "uuid-contract-token",
		Status:       model.TokenStatusEnabled,
		CreatedTime:  helper.GetTimestamp(),
		AccessedTime: helper.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(token).Error)
	require.NotEmpty(t, token.UUID)

	redemption := &model.Redemption{
		UserId:      user.Id,
		UserUUID:    &user.UUID,
		Key:         "uuid-contract-redemption-key",
		Name:        "uuid-contract-redemption",
		CreatedTime: helper.GetTimestamp(),
		Quota:       100,
		Status:      model.RedemptionCodeStatusEnabled,
	}
	require.NoError(t, model.DB.Create(redemption).Error)
	require.NotEmpty(t, redemption.UUID)

	mcpServer := &model.MCPServer{
		Name:     "uuid-contract-mcp",
		BaseURL:  "https://example.com/mcp",
		Protocol: model.MCPProtocolStreamableHTTP,
		AuthType: model.MCPAuthTypeNone,
		Status:   model.MCPServerStatusEnabled,
	}
	require.NoError(t, model.DB.Create(mcpServer).Error)
	require.NotEmpty(t, mcpServer.UUID)

	mcpTool := &model.MCPTool{
		Name:        "uuid_contract_tool",
		DisplayName: "UUID Contract Tool",
		Description: "tool used by uuid contract tests",
		Status:      1,
	}
	require.NoError(t, model.UpsertMCPTools(mcpServer.Id, mcpServer.UUID, []*model.MCPTool{mcpTool}))
	require.NotEmpty(t, mcpTool.UUID)
	require.NotNil(t, mcpTool.ServerUUID)

	trace := &model.Trace{
		TraceId:    "uuid-contract-trace",
		URL:        "/api/test",
		Method:     http.MethodGet,
		Timestamps: `{"request_received":1,"request_completed":4}`,
	}
	require.NoError(t, model.DB.Create(trace).Error)
	require.NotEmpty(t, trace.UUID)

	log := &model.Log{
		UserId:      user.Id,
		UserUUID:    &user.UUID,
		ChannelId:   channel.Id,
		ChannelUUID: &channel.UUID,
		TokenName:   token.Name,
		TokenUUID:   &token.UUID,
		TraceId:     trace.TraceId,
		Type:        model.LogTypeConsume,
		Content:     "uuid contract log",
		Username:    user.Username,
	}
	require.NoError(t, model.DB.Create(log).Error)
	require.NotEmpty(t, log.UUID)

	cost := &model.UserRequestCost{
		CreatedTime: helper.GetTimestamp(),
		UserID:      user.Id,
		UserUUID:    &user.UUID,
		RequestID:   "uuid-contract-request",
		Quota:       2500,
	}
	require.NoError(t, model.DB.Create(cost).Error)
	require.NotEmpty(t, cost.UUID)

	passkey := &model.PasskeyCredential{
		UserId:         user.Id,
		UserUUID:       &user.UUID,
		CredentialName: "uuid-contract-passkey",
		CredentialID:   []byte("uuid-contract-passkey-credential"),
		PublicKey:      []byte("uuid-contract-passkey-public-key"),
		SignCount:      7,
	}
	require.NoError(t, model.DB.Create(passkey).Error)
	require.NotEmpty(t, passkey.UUID)

	fixture := &uuidContractFixture{
		user:       user,
		token:      token,
		channel:    channel,
		redemption: redemption,
		mcpServer:  mcpServer,
		mcpTool:    mcpTool,
		trace:      trace,
		log:        log,
		cost:       cost,
		passkey:    passkey,
	}
	cleanup := func() {
		model.DB = originalDB
		model.LOG_DB = originalLOGDB
		common.UsingSQLite.Store(originalUsingSQLite)
	}
	return fixture, cleanup
}

// decodeUUIDContractData decodes a standard controller response and returns the data object.
// Parameters:
//   - t: active test handle.
//   - recorder: HTTP response recorder populated by the handler.
//
// Return values:
//   - map[string]any: decoded response data object.
func decodeUUIDContractData(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, true, payload["success"])

	data, ok := payload["data"].(map[string]any)
	require.True(t, ok, "data must be a JSON object: %s", recorder.Body.String())
	return data
}

// TestUUIDDetailEndpointsAcceptUUIDAndEmitStageIdentifiers verifies representative T-A endpoints.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestUUIDDetailEndpointsAcceptUUIDAndEmitStageIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/user/:id", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		GetUser(c)
	})
	router.GET("/api/channel/:id", func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		GetChannel(c)
	})
	router.GET("/api/token/:id", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		GetToken(c)
	})
	router.GET("/api/redemption/:id", GetRedemption)
	router.GET("/api/mcp_servers/:id", GetMCPServer)

	tests := []struct {
		name string
		path string
		uuid string
	}{
		{name: "user", path: "/api/user/" + fixture.user.UUID, uuid: fixture.user.UUID},
		{name: "channel", path: "/api/channel/" + fixture.channel.UUID, uuid: fixture.channel.UUID},
		{name: "token", path: "/api/token/" + fixture.token.UUID, uuid: fixture.token.UUID},
		{name: "redemption", path: "/api/redemption/" + fixture.redemption.UUID, uuid: fixture.redemption.UUID},
		{name: "mcp_server", path: "/api/mcp_servers/" + fixture.mcpServer.UUID, uuid: fixture.mcpServer.UUID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			router.ServeHTTP(recorder, request)

			data := decodeUUIDContractData(t, recorder)
			require.Equal(t, tt.uuid, data["uuid"])
			require.NotContains(t, data, "id")
			require.NotContains(t, data, "user_id")
			if tt.name == "token" {
				require.Equal(t, fixture.user.UUID, data["user_uuid"])
			}
		})
	}
}

// TestUserStrictOutResponses verifies user S2 responses keep UUIDs and hide internal integer IDs.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestUserStrictOutResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/user/", GetAllUsers)
	router.GET("/api/user/search", SearchUsers)
	router.GET("/api/user/:id", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		GetUser(c)
	})
	router.GET("/api/dashboard/users", func(c *gin.Context) {
		c.Set(ctxkey.Role, model.RoleRootUser)
		GetDashboardUsers(c)
	})

	detailRecorder := httptest.NewRecorder()
	router.ServeHTTP(detailRecorder, httptest.NewRequest(http.MethodGet, "/api/user/"+fixture.user.UUID, nil))
	detail := decodeUUIDContractData(t, detailRecorder)
	require.Equal(t, fixture.user.UUID, detail["uuid"])
	require.NotContains(t, detail, "id")
	require.NotContains(t, detail, "inviter_id")
	require.NotContains(t, detail, "password")
	require.NotContains(t, detail, "access_token")

	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/api/user/?p=0&size=10", nil))
	var listPayload map[string]any
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &listPayload))
	require.Equal(t, true, listPayload["success"])
	listData, ok := listPayload["data"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, listData)
	listUser, ok := listData[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, fixture.user.UUID, listUser["uuid"])
	require.NotContains(t, listUser, "id")
	require.NotContains(t, listUser, "inviter_id")

	searchRecorder := httptest.NewRecorder()
	router.ServeHTTP(searchRecorder, httptest.NewRequest(http.MethodGet, "/api/user/search?keyword=uuid-contract", nil))
	var searchPayload map[string]any
	require.NoError(t, json.Unmarshal(searchRecorder.Body.Bytes(), &searchPayload))
	require.Equal(t, true, searchPayload["success"])
	searchData, ok := searchPayload["data"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, searchData)
	searchUser, ok := searchData[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, fixture.user.UUID, searchUser["uuid"])
	require.NotContains(t, searchUser, "id")
	require.NotContains(t, searchUser, "inviter_id")

	dashboardRecorder := httptest.NewRecorder()
	router.ServeHTTP(dashboardRecorder, httptest.NewRequest(http.MethodGet, "/api/dashboard/users", nil))
	var dashboardPayload map[string]any
	require.NoError(t, json.Unmarshal(dashboardRecorder.Body.Bytes(), &dashboardPayload))
	require.Equal(t, true, dashboardPayload["success"])
	dashboardData, ok := dashboardPayload["data"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, dashboardData)
	for _, item := range dashboardData {
		userOption, ok := item.(map[string]any)
		require.True(t, ok)
		require.NotContains(t, userOption, "id")
		require.NotEmpty(t, userOption["uuid"])
	}
}

// TestRedemptionStrictOutResponses verifies redemption strict-out responses and strict-in request handling.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestRedemptionStrictOutResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/redemption/", GetAllRedemptions)
	router.GET("/api/redemption/search", SearchRedemptions)
	router.GET("/api/redemption/:id", GetRedemption)
	router.PUT("/api/redemption/", UpdateRedemption)
	router.POST("/api/redemption/", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		c.Set(ctxkey.UserUUID, fixture.user.UUID)
		AddRedemption(c)
	})

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/redemption/?p=0&size=10", nil)
	router.ServeHTTP(listRecorder, listRequest)
	require.Equal(t, http.StatusOK, listRecorder.Code)
	var listPayload map[string]any
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &listPayload))
	rows, ok := listPayload["data"].([]any)
	require.True(t, ok, "redemption list data must be an array: %s", listRecorder.Body.String())
	require.NotEmpty(t, rows)
	row, ok := rows[0].(map[string]any)
	require.True(t, ok, "redemption list row must be an object")
	require.Equal(t, fixture.redemption.UUID, row["uuid"])
	require.Equal(t, fixture.user.UUID, row["user_uuid"])
	require.NotContains(t, row, "id")
	require.NotContains(t, row, "user_id")

	searchRecorder := httptest.NewRecorder()
	searchRequest := httptest.NewRequest(http.MethodGet, "/api/redemption/search?keyword=uuid-contract-redemption&p=0&size=10", nil)
	router.ServeHTTP(searchRecorder, searchRequest)
	require.Equal(t, http.StatusOK, searchRecorder.Code)
	var searchPayload map[string]any
	require.NoError(t, json.Unmarshal(searchRecorder.Body.Bytes(), &searchPayload))
	searchRows, ok := searchPayload["data"].([]any)
	require.True(t, ok, "redemption search data must be an array: %s", searchRecorder.Body.String())
	require.NotEmpty(t, searchRows)
	searchRow, ok := searchRows[0].(map[string]any)
	require.True(t, ok, "redemption search row must be an object")
	require.Equal(t, fixture.redemption.UUID, searchRow["uuid"])
	require.Equal(t, fixture.user.UUID, searchRow["user_uuid"])
	require.NotContains(t, searchRow, "id")
	require.NotContains(t, searchRow, "user_id")

	detailRecorder := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(http.MethodGet, "/api/redemption/"+fixture.redemption.UUID, nil)
	router.ServeHTTP(detailRecorder, detailRequest)
	detailData := decodeUUIDContractData(t, detailRecorder)
	require.Equal(t, fixture.redemption.UUID, detailData["uuid"])
	require.Equal(t, fixture.user.UUID, detailData["user_uuid"])
	require.NotContains(t, detailData, "id")
	require.NotContains(t, detailData, "user_id")

	updateRecorder := httptest.NewRecorder()
	updateBody := `{"uuid":"` + fixture.redemption.UUID + `","status":2}`
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/redemption/?status_only=true", strings.NewReader(updateBody))
	updateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRecorder, updateRequest)
	updateData := decodeUUIDContractData(t, updateRecorder)
	require.Equal(t, fixture.redemption.UUID, updateData["uuid"])
	require.Equal(t, fixture.user.UUID, updateData["user_uuid"])
	require.EqualValues(t, model.RedemptionCodeStatusDisabled, updateData["status"])
	require.NotContains(t, updateData, "id")
	require.NotContains(t, updateData, "user_id")

	legacyUpdateRecorder := httptest.NewRecorder()
	legacyUpdateRequest := httptest.NewRequest(http.MethodPut, "/api/redemption/?status_only=true", strings.NewReader(`{"id":1,"status":1}`))
	legacyUpdateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(legacyUpdateRecorder, legacyUpdateRequest)
	var legacyUpdatePayload map[string]any
	require.NoError(t, json.Unmarshal(legacyUpdateRecorder.Body.Bytes(), &legacyUpdatePayload))
	require.Equal(t, false, legacyUpdatePayload["success"])

	createRecorder := httptest.NewRecorder()
	createBody := `{"name":"created-redemption","count":1,"quota":100,"uuid":"018f0000-0000-7000-8000-00000000feed","user_uuid":"018f0000-0000-7000-8000-00000000feed"}`
	createRequest := httptest.NewRequest(http.MethodPost, "/api/redemption/", strings.NewReader(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRecorder, createRequest)
	require.Equal(t, http.StatusOK, createRecorder.Code)
	var createPayload map[string]any
	require.NoError(t, json.Unmarshal(createRecorder.Body.Bytes(), &createPayload))
	require.Equal(t, true, createPayload["success"])
	keys, ok := createPayload["data"].([]any)
	require.True(t, ok, "redemption create data must be an array: %s", createRecorder.Body.String())
	require.Len(t, keys, 1)

	var created model.Redemption
	require.NoError(t, model.DB.Where("name = ?", "created-redemption").First(&created).Error)
	require.NotEqual(t, "018f0000-0000-7000-8000-00000000feed", created.UUID)
	require.Equal(t, fixture.user.UUID, *created.UserUUID)
}

// TestMCPServerStrictOutResponses verifies MCP server and MCP tool S2 response contracts.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestMCPServerStrictOutResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/mcp_servers", GetMCPServers)
	router.GET("/api/mcp_servers/", GetMCPServers)
	router.GET("/api/mcp_servers/:id", GetMCPServer)
	router.POST("/api/mcp_servers/", func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		CreateMCPServer(c)
	})
	router.PUT("/api/mcp_servers/:id", func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		UpdateMCPServer(c)
	})
	router.GET("/api/mcp_servers/:id/tools", func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		ListMCPServerTools(c)
	})
	router.GET("/api/mcp_tools", GetMCPTools)
	router.GET("/api/tools/display", GetToolsDisplay)

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/mcp_servers/?p=0&size=10", nil)
	router.ServeHTTP(listRecorder, listRequest)
	require.Equal(t, http.StatusOK, listRecorder.Code)

	var listPayload map[string]any
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &listPayload))
	rows, ok := listPayload["data"].([]any)
	require.True(t, ok, "mcp server list data must be an array: %s", listRecorder.Body.String())
	require.NotEmpty(t, rows)
	row, ok := rows[0].(map[string]any)
	require.True(t, ok, "mcp server list row must be an object")
	server, ok := row["server"].(map[string]any)
	require.True(t, ok, "mcp server list row must contain server object")
	require.Equal(t, fixture.mcpServer.UUID, server["uuid"])
	require.NotContains(t, server, "id")

	detailRecorder := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(http.MethodGet, "/api/mcp_servers/"+fixture.mcpServer.UUID, nil)
	router.ServeHTTP(detailRecorder, detailRequest)
	detailData := decodeUUIDContractData(t, detailRecorder)
	require.Equal(t, fixture.mcpServer.UUID, detailData["uuid"])
	require.NotContains(t, detailData, "id")

	updateRecorder := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/mcp_servers/"+fixture.mcpServer.UUID, strings.NewReader(`{"description":"updated strict-out"}`))
	updateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRecorder, updateRequest)
	updateData := decodeUUIDContractData(t, updateRecorder)
	require.Equal(t, fixture.mcpServer.UUID, updateData["uuid"])
	require.Equal(t, "updated strict-out", updateData["description"])
	require.NotContains(t, updateData, "id")

	legacyUpdateRecorder := httptest.NewRecorder()
	legacyUpdateRequest := httptest.NewRequest(http.MethodPut, "/api/mcp_servers/1", strings.NewReader(`{"description":"legacy int update"}`))
	legacyUpdateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(legacyUpdateRecorder, legacyUpdateRequest)
	var legacyUpdatePayload map[string]any
	require.NoError(t, json.Unmarshal(legacyUpdateRecorder.Body.Bytes(), &legacyUpdatePayload))
	require.Equal(t, false, legacyUpdatePayload["success"])

	createRecorder := httptest.NewRecorder()
	createBody := `{"name":"uuid-contract-created-mcp","base_url":"https://created.example.com/mcp","protocol":"streamable_http","auth_type":"none","uuid":"018f0000-0000-7000-8000-00000000feed"}`
	createRequest := httptest.NewRequest(http.MethodPost, "/api/mcp_servers/", strings.NewReader(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRecorder, createRequest)
	createData := decodeUUIDContractData(t, createRecorder)
	require.NotEmpty(t, createData["uuid"])
	require.NotEqual(t, "018f0000-0000-7000-8000-00000000feed", createData["uuid"])
	require.NotContains(t, createData, "id")

	toolListRecorder := httptest.NewRecorder()
	toolListRequest := httptest.NewRequest(http.MethodGet, "/api/mcp_servers/"+fixture.mcpServer.UUID+"/tools", nil)
	router.ServeHTTP(toolListRecorder, toolListRequest)
	require.Equal(t, http.StatusOK, toolListRecorder.Code)

	var toolListPayload map[string]any
	require.NoError(t, json.Unmarshal(toolListRecorder.Body.Bytes(), &toolListPayload))
	tools, ok := toolListPayload["data"].([]any)
	require.True(t, ok, "mcp server tools data must be an array: %s", toolListRecorder.Body.String())
	require.NotEmpty(t, tools)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok, "mcp tool row must be an object")
	require.Equal(t, fixture.mcpTool.UUID, tool["uuid"])
	require.Equal(t, fixture.mcpServer.UUID, tool["server_uuid"])
	require.NotContains(t, tool, "id")
	require.NotContains(t, tool, "server_id")

	toolSearchRecorder := httptest.NewRecorder()
	toolSearchRequest := httptest.NewRequest(http.MethodGet, "/api/mcp_tools?server_id="+fixture.mcpServer.UUID, nil)
	router.ServeHTTP(toolSearchRecorder, toolSearchRequest)
	require.Equal(t, http.StatusOK, toolSearchRecorder.Code)
	var toolSearchPayload map[string]any
	require.NoError(t, json.Unmarshal(toolSearchRecorder.Body.Bytes(), &toolSearchPayload))
	filteredTools, ok := toolSearchPayload["data"].([]any)
	require.True(t, ok, "mcp tools data must be an array: %s", toolSearchRecorder.Body.String())
	require.Len(t, filteredTools, 1)
	filteredTool, ok := filteredTools[0].(map[string]any)
	require.True(t, ok, "filtered mcp tool row must be an object")
	require.Equal(t, fixture.mcpTool.UUID, filteredTool["uuid"])
	require.Equal(t, fixture.mcpServer.UUID, filteredTool["server_uuid"])
	require.NotContains(t, filteredTool, "id")
	require.NotContains(t, filteredTool, "server_id")

	displayRecorder := httptest.NewRecorder()
	displayRequest := httptest.NewRequest(http.MethodGet, "/api/tools/display", nil)
	router.ServeHTTP(displayRecorder, displayRequest)
	require.Equal(t, http.StatusOK, displayRecorder.Code)
	var displayPayload map[string]any
	require.NoError(t, json.Unmarshal(displayRecorder.Body.Bytes(), &displayPayload))
	displayRows, ok := displayPayload["data"].([]any)
	require.True(t, ok, "tools display data must be an array: %s", displayRecorder.Body.String())
	require.NotEmpty(t, displayRows)
	displayRow, ok := displayRows[0].(map[string]any)
	require.True(t, ok, "tools display row must be an object")
	displayServer, ok := displayRow["server"].(map[string]any)
	require.True(t, ok, "tools display row must contain server object")
	require.Equal(t, fixture.mcpServer.UUID, displayServer["uuid"])
	require.NotContains(t, displayServer, "id")
	displayTools, ok := displayRow["tools"].([]any)
	require.True(t, ok, "tools display row must contain tools array")
	require.NotEmpty(t, displayTools)
	displayTool, ok := displayTools[0].(map[string]any)
	require.True(t, ok, "tools display tool must be an object")
	require.Equal(t, fixture.mcpTool.UUID, displayTool["uuid"])
	require.Equal(t, fixture.mcpServer.UUID, displayTool["server_uuid"])
	require.NotContains(t, displayTool, "id")
	require.NotContains(t, displayTool, "server_id")
}

// TestTokenStrictInResponses verifies token strict-out responses and strict-in request handling.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestTokenStrictInResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/token/:id", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		GetToken(c)
	})
	router.PUT("/api/token/", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		UpdateToken(c)
	})

	getRecorder := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/token/"+fixture.token.UUID, nil)
	router.ServeHTTP(getRecorder, getRequest)
	getData := decodeUUIDContractData(t, getRecorder)
	require.Equal(t, fixture.token.UUID, getData["uuid"])
	require.Equal(t, fixture.user.UUID, getData["user_uuid"])
	require.NotContains(t, getData, "id")
	require.NotContains(t, getData, "user_id")

	updateRecorder := httptest.NewRecorder()
	updateBody := `{"uuid":"` + fixture.token.UUID + `","status":2}`
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/token/?status_only=1", strings.NewReader(updateBody))
	updateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateRecorder, updateRequest)
	updateData := decodeUUIDContractData(t, updateRecorder)
	require.Equal(t, fixture.token.UUID, updateData["uuid"])
	require.Equal(t, fixture.user.UUID, updateData["user_uuid"])
	require.EqualValues(t, model.TokenStatusDisabled, updateData["status"])
	require.NotContains(t, updateData, "id")
	require.NotContains(t, updateData, "user_id")

	legacyGetRecorder := httptest.NewRecorder()
	legacyGetRequest := httptest.NewRequest(http.MethodGet, "/api/token/1", nil)
	router.ServeHTTP(legacyGetRecorder, legacyGetRequest)
	var legacyGetPayload map[string]any
	require.NoError(t, json.Unmarshal(legacyGetRecorder.Body.Bytes(), &legacyGetPayload))
	require.Equal(t, false, legacyGetPayload["success"])

	legacyUpdateRecorder := httptest.NewRecorder()
	legacyUpdateRequest := httptest.NewRequest(http.MethodPut, "/api/token/?status_only=1", strings.NewReader(`{"id":1,"status":1}`))
	legacyUpdateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(legacyUpdateRecorder, legacyUpdateRequest)
	var legacyUpdatePayload map[string]any
	require.NoError(t, json.Unmarshal(legacyUpdateRecorder.Body.Bytes(), &legacyUpdatePayload))
	require.Equal(t, false, legacyUpdatePayload["success"])
}

// TestTokenStrictOutListSearchAndCreateResponses verifies token S2 on non-detail token responses.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestTokenStrictOutListSearchAndCreateResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/token/", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		GetAllTokens(c)
	})
	router.GET("/api/token/search", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		SearchTokens(c)
	})
	router.POST("/api/token/", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		AddToken(c)
	})

	for _, path := range []string{"/api/token/?p=0&size=10", "/api/token/search?keyword=uuid-contract-token&p=0&size=10"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)

		var payload map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		require.Equal(t, true, payload["success"])
		rows, ok := payload["data"].([]any)
		require.True(t, ok, "data must be a JSON array: %s", recorder.Body.String())
		require.NotEmpty(t, rows)
		row, ok := rows[0].(map[string]any)
		require.True(t, ok, "token row must be a JSON object: %s", recorder.Body.String())
		require.Equal(t, fixture.token.UUID, row["uuid"])
		require.Equal(t, fixture.user.UUID, row["user_uuid"])
		require.NotContains(t, row, "id")
		require.NotContains(t, row, "user_id")
	}

	createRecorder := httptest.NewRecorder()
	createBody := `{"name":"created-token","remain_quota":10,"expired_time":-1,"uuid":"018f0000-0000-7000-8000-00000000feed","user_uuid":"018f0000-0000-7000-8000-00000000feed"}`
	createRequest := httptest.NewRequest(http.MethodPost, "/api/token/", strings.NewReader(createBody))
	createRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRecorder, createRequest)
	createData := decodeUUIDContractData(t, createRecorder)
	require.NotEmpty(t, createData["uuid"])
	require.NotEqual(t, "018f0000-0000-7000-8000-00000000feed", createData["uuid"])
	require.Equal(t, fixture.user.UUID, createData["user_uuid"])
	require.NotContains(t, createData, "id")
	require.NotContains(t, createData, "user_id")
}

// TestTraceAndRequestCostStrictOutResponses verifies remaining serialized-only strict-out responses.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestTraceAndRequestCostStrictOutResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/cost/request/:request_id", GetRequestCost)
	router.GET("/api/trace/:trace_id", func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		GetTraceByTraceId(c)
	})
	router.GET("/api/trace/log/:log_id", func(c *gin.Context) {
		gmwSetLoggerForUUIDContract(c)
		GetTraceByLogId(c)
	})

	costRecorder := httptest.NewRecorder()
	costRequest := httptest.NewRequest(http.MethodGet, "/api/cost/request/"+fixture.cost.RequestID, nil)
	router.ServeHTTP(costRecorder, costRequest)
	require.Equal(t, http.StatusOK, costRecorder.Code)
	var costData map[string]any
	require.NoError(t, json.Unmarshal(costRecorder.Body.Bytes(), &costData))
	require.Equal(t, fixture.cost.UUID, costData["uuid"])
	require.Equal(t, fixture.user.UUID, costData["user_uuid"])
	require.EqualValues(t, fixture.cost.Quota, costData["quota"])
	require.EqualValues(t, float64(fixture.cost.Quota)/500000, costData["cost_usd"])
	require.NotContains(t, costData, "id")
	require.NotContains(t, costData, "user_id")

	traceRecorder := httptest.NewRecorder()
	traceRequest := httptest.NewRequest(http.MethodGet, "/api/trace/"+fixture.trace.TraceId, nil)
	router.ServeHTTP(traceRecorder, traceRequest)
	traceData := decodeUUIDContractData(t, traceRecorder)
	require.Equal(t, fixture.trace.UUID, traceData["uuid"])
	require.Equal(t, fixture.trace.TraceId, traceData["trace_id"])
	require.NotContains(t, traceData, "id")

	traceByLogRecorder := httptest.NewRecorder()
	traceByLogRequest := httptest.NewRequest(http.MethodGet, "/api/trace/log/"+fixture.log.UUID, nil)
	router.ServeHTTP(traceByLogRecorder, traceByLogRequest)
	traceByLogData := decodeUUIDContractData(t, traceByLogRecorder)
	require.Equal(t, fixture.trace.UUID, traceByLogData["uuid"])
	require.Equal(t, fixture.trace.TraceId, traceByLogData["trace_id"])
	require.NotContains(t, traceByLogData, "id")

	logData, ok := traceByLogData["log"].(map[string]any)
	require.True(t, ok, "trace-by-log response must contain a log object")
	require.Equal(t, fixture.log.UUID, logData["uuid"])
	require.Equal(t, fixture.user.UUID, logData["user_uuid"])
	require.Equal(t, fixture.channel.UUID, logData["channel_uuid"])
	require.NotContains(t, logData, "id")
	require.NotContains(t, logData, "user_id")
}

// gmwSetLoggerForUUIDContract attaches a logger for handlers that read gin-middlewares logger state.
// Parameters:
//   - c: gin request context.
//
// Return values: none.
func gmwSetLoggerForUUIDContract(c *gin.Context) {
	gmw.SetLogger(c, logger.Logger)
}
