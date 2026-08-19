package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// createMCPServerForDuplicateTest inserts a server row used by controller
// duplicate-name behavior tests.
func createMCPServerForDuplicateTest(t *testing.T, name string, baseURL string) *model.MCPServer {
	t.Helper()
	server := &model.MCPServer{
		Name:     name,
		BaseURL:  baseURL,
		Protocol: model.MCPProtocolStreamableHTTP,
		AuthType: model.MCPAuthTypeNone,
		Status:   model.MCPServerStatusEnabled,
	}
	require.NoError(t, model.DB.Create(server).Error)
	return server
}

// TestCreateMCPServerDuplicateNameReturnsPublicFailure verifies duplicate MCP
// server names do not expose raw database unique-constraint errors.
func TestCreateMCPServerDuplicateNameReturnsPublicFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	teardown := setupEmptyValueMCPServerEnv(t)
	defer teardown()
	createMCPServerForDuplicateTest(t, "taken-mcp-create", "https://example.com/create")

	payload := map[string]any{
		"name":      "taken-mcp-create",
		"base_url":  "https://example.com/create-new",
		"protocol":  model.MCPProtocolStreamableHTTP,
		"auth_type": model.MCPAuthTypeNone,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/mcp_servers/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newMCPServerRouter().ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, false, resp["success"])
	require.Equal(t, "MCP server name already exists", resp["message"])
	require.NotContains(t, w.Body.String(), "unique constraint")
	require.NotContains(t, w.Body.String(), "duplicate key")

	var count int64
	require.NoError(t, model.DB.Model(&model.MCPServer{}).Where("name = ?", "taken-mcp-create").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// TestUpdateMCPServerDuplicateNameReturnsPublicFailure verifies MCP server
// rename collisions return a stable public response.
func TestUpdateMCPServerDuplicateNameReturnsPublicFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	teardown := setupEmptyValueMCPServerEnv(t)
	defer teardown()
	createMCPServerForDuplicateTest(t, "taken-mcp-update", "https://example.com/update-taken")
	target := createMCPServerForDuplicateTest(t, "mcp-update-target", "https://example.com/update-target")

	payload := map[string]any{
		"name":      "taken-mcp-update",
		"base_url":  "https://example.com/update-target",
		"protocol":  model.MCPProtocolStreamableHTTP,
		"auth_type": model.MCPAuthTypeNone,
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/mcp_servers/"+target.UUID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newMCPServerRouter().ServeHTTP(w, req)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, false, resp["success"])
	require.Equal(t, "MCP server name already exists", resp["message"])
	require.NotContains(t, w.Body.String(), "unique constraint")
	require.NotContains(t, w.Body.String(), "duplicate key")

	var updated model.MCPServer
	require.NoError(t, model.DB.First(&updated, target.Id).Error)
	require.Equal(t, "mcp-update-target", updated.Name)
}
