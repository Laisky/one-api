package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// TestMCPProxyLatestDiscover verifies modern clients can discover protocol support without initialize.
func TestMCPProxyLatestDiscover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", MCPProxyLatest)

	body := modernMCPRequestBody(t, "discover-1", "server/discover", map[string]any{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	request.Header.Set(mcp.MethodHeader, "server/discover")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var envelope struct {
		Result map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, mcp.ResultTypeComplete, envelope.Result["resultType"])
	require.Equal(t, []any{mcp.ProtocolVersion}, envelope.Result["supportedVersions"])
	meta := envelope.Result["_meta"].(map[string]any)
	serverInfo := meta[mcp.MetaServerInfoKey].(map[string]any)
	require.Equal(t, mcpServerName, serverInfo["name"])
	require.Equal(t, mcp.CacheScopePrivate, envelope.Result["cacheScope"])
	require.NotZero(t, envelope.Result["ttlMs"])
}

// TestMCPProxyLatestRejectsHeaderMismatch verifies modern mirrored protocol fields are enforced.
func TestMCPProxyLatestRejectsHeaderMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", MCPProxyLatest)

	body := modernMCPRequestBody(t, "mismatch-1", "server/discover", map[string]any{})
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(mcp.ProtocolVersionHeader, "2026-01-01")
	request.Header.Set(mcp.MethodHeader, "server/discover")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	var envelope struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, mcp.ErrorCodeHeaderMismatch, envelope.Error.Code)
}

// TestMCPProxyLatestRejectsCrossOrigin verifies modern browser requests cannot target a mismatched host.
func TestMCPProxyLatestRejectsCrossOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", MCPProxyLatest)

	body := modernMCPRequestBody(t, "origin-1", "server/discover", map[string]any{})
	request := httptest.NewRequest(http.MethodPost, "https://gateway.example/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	request.Header.Set(mcp.MethodHeader, "server/discover")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
}

// TestMCPProxyLatestDelegatesLegacyInitialize verifies pre-2026 clients retain the existing lifecycle.
func TestMCPProxyLatestDelegatesLegacyInitialize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", MCPProxyLatest)

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "legacy-1",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": mcp.LegacyProtocolVersionFallback,
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "legacy", "version": "1"},
		},
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var envelope struct {
		Result map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, mcpProtocolVersion, envelope.Result["protocolVersion"])
}

// TestMCPProxyLatestClientServerContract verifies one-api's modern client can drive its own modern server.
func TestMCPProxyLatestClientServerContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", MCPProxyLatest)
	server := httptest.NewServer(router)
	defer server.Close()

	client := mcp.NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL + "/mcp"}, nil, 5*time.Second)
	result, err := client.DiscoverLatest(context.Background())
	require.NoError(t, err)
	require.Equal(t, mcp.ResultTypeComplete, result.ResultType)
	require.Equal(t, []string{mcp.ProtocolVersion}, result.SupportedVersions)
	require.Equal(t, mcpServerName, result.Meta.ServerInfo.Name)
}

// modernMCPRequestBody builds a protocol-complete request body for controller tests.
func modernMCPRequestBody(t *testing.T, id, method string, params map[string]any) []byte {
	t.Helper()
	params = mcp.WithModernMeta(params)
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	require.NoError(t, err)
	return body
}

// TestValidateModernMCPRequestAcceptsEncodedName verifies mirrored tool names are decoded before comparison.
func TestValidateModernMCPRequestAcceptsEncodedName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	params := mcp.WithModernMeta(map[string]any{
		"name":      "天气",
		"arguments": map[string]any{},
	})
	encoded, err := json.Marshal(params)
	require.NoError(t, err)
	context.Request = httptest.NewRequest(http.MethodPost, "https://gateway.example/mcp", nil)
	context.Request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	context.Request.Header.Set(mcp.MethodHeader, "tools/call")
	context.Request.Header.Set(mcp.NameHeader, mcp.EncodeMCPHeaderValue("天气"))

	err = validateModernMCPRequest(context, mcpRPCRequest{
		JSONRPC: "2.0",
		ID:      "call-1",
		Method:  "tools/call",
		Params:  encoded,
	})
	require.NoError(t, err)
}

// TestMCPProxyLatestRejectsMissingClientCapabilities verifies every modern request is self-contained.
func TestMCPProxyLatestRejectsMissingClientCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", MCPProxyLatest)

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "missing-capabilities",
		"method":  "server/discover",
		"params": map[string]any{
			"_meta": map[string]any{
				mcp.MetaProtocolVersionKey: mcp.ProtocolVersion,
			},
		},
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	request.Header.Set(mcp.MethodHeader, "server/discover")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code)
	var envelope struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, mcpErrInvalidRequest, envelope.Error.Code)
}
