package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/model"
)

// setupMCPToolsTestDB creates an in-memory database for MCP tool tests.
func setupMCPToolsTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&model.MCPServer{}, &model.MCPTool{})
	require.NoError(t, err)

	return db
}

// setupMCPToolsTestEnvironment wires the test database into the model layer.
func setupMCPToolsTestEnvironment(t *testing.T) func() {
	testDB := setupMCPToolsTestDB(t)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite.Load()

	model.DB = testDB
	model.LOG_DB = testDB
	common.UsingSQLite.Store(true)

	return func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite.Store(originalUsingSQLite)
	}
}

// TestListMCPServerToolsAppliesPricing ensures MCP tool pricing is included in the tools list response.
func TestListMCPServerToolsAppliesPricing(t *testing.T) {
	cleanup := setupMCPToolsTestEnvironment(t)
	defer cleanup()

	server := &model.MCPServer{
		Name:        "test-mcp",
		BaseURL:     "https://example.com",
		Protocol:    model.MCPProtocolStreamableHTTP,
		AuthType:    model.MCPAuthTypeNone,
		ToolPricing: model.MCPToolPricingMap{"web_fetch": {UsdPerCall: 0.001}, "WEB_SEARCH": {UsdPerCall: 0.01}},
	}
	require.NoError(t, model.CreateMCPServer(server))

	require.NoError(t, model.UpsertMCPTools(server.Id, []*model.MCPTool{
		{Name: "web_fetch", DisplayName: "web_fetch"},
		{Name: "web_search", DisplayName: "web_search"},
		{Name: "free_tool", DisplayName: "free_tool", InputSchema: "null"},
	}))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/mcp_servers/:id/tools", ListMCPServerTools)

	request, _ := http.NewRequest(http.MethodGet, "/api/mcp_servers/"+strconv.Itoa(server.Id)+"/tools", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)

	var payload struct {
		Success bool            `json:"success"`
		Data    []model.MCPTool `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	tools := make(map[string]model.MCPTool, len(payload.Data))
	for _, tool := range payload.Data {
		tools[tool.Name] = tool
	}

	fetchPricing := model.ToolPricingLocal(tools["web_fetch"].DefaultPricing)
	searchPricing := model.ToolPricingLocal(tools["web_search"].DefaultPricing)
	freePricing := model.ToolPricingLocal(tools["free_tool"].DefaultPricing)
	freeSchema := tools["free_tool"].InputSchema

	require.InDelta(t, 0.001, fetchPricing.UsdPerCall, 1e-9)
	require.InDelta(t, 0.01, searchPricing.UsdPerCall, 1e-9)
	require.Equal(t, int64(0), freePricing.QuotaPerCall)
	require.InDelta(t, 0.0, freePricing.UsdPerCall, 1e-9)
	require.Equal(t, "", freeSchema)
}
