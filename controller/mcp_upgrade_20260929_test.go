package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// TestMCP20260929GatewayMetadataErrors verifies required metadata uses Invalid params, not a header error.
func TestMCP20260929GatewayMetadataErrors(t *testing.T) {
	for _, meta := range []string{`null`, `{}`, `{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}`, `{"io.modelcontextprotocol/clientCapabilities":{}}`} {
		t.Run(meta, func(t *testing.T) {
			router := gin.New()
			router.POST("/mcp", MCPProxyLatest)
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":9007199254740993,"method":"server/discover","params":{"_meta":%s}}`, meta)
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
			request.Header.Set(mcp.MethodHeader, "server/discover")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, response.Body.String(), `"code":-32602`)
			require.Contains(t, response.Body.String(), `"id":9007199254740993`)
		})
	}
}

// TestMCP20260929GatewayIdentityPrecision verifies valid large IDs are echoed exactly and fractional IDs are rejected.
func TestMCP20260929GatewayIdentityPrecision(t *testing.T) {
	for _, id := range []string{`9007199254740993`, `18446744073709551615`, `9007199254740993.5`} {
		t.Run(id, func(t *testing.T) {
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`, id)
			router := gin.New()
			router.POST("/mcp", MCPProxyLatest)
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
			request.Header.Set(mcp.MethodHeader, "server/discover")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if strings.Contains(id, ".") {
				require.Equal(t, http.StatusBadRequest, response.Code)
				require.Contains(t, response.Body.String(), `"id":null`)
			} else {
				require.Equal(t, http.StatusOK, response.Code)
				require.Contains(t, response.Body.String(), `"id":`+id)
			}
		})
	}
}

// TestMCP20260929GatewayElicitation exercises actual HTTP, catalog persistence, MRTR state and final-only auditing.
func TestMCP20260929GatewayElicitation(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()
	var upstreamCalls atomic.Int64
	observed := make(chan string, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		observed <- string(body)
		if r.Header.Get("Authorization") != "" {
			t.Error("gateway caller credential reached upstream")
		}
		var request struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
			return
		}
		round := upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		result := `{"resultType":"input_required","inputRequests":{"approval":{"method":"elicitation/create","params":{"mode":"url","message":"Confirm operation","url":"https://approval.example/consent"}}},"requestState":"opaque state"}`
		if round > 1 {
			result = `{"resultType":"complete","content":[],"structuredContent":{"revision":9007199254740993},"_meta":{"com.example/revision":9007199254740993}}`
		}
		if _, err := fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, request.ID, result); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()
	require.NoError(t, model.DB.Model(&model.MCPServer{}).Where("id = ?", fx.server.Id).Update("base_url", upstream.URL).Error)
	meta := `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{"elicitation":{"url":{}},"extensions":{"io.modelcontextprotocol/tasks":{}}},"traceparent":"00-0af7651916cd43dd8448eb211c80319c-00f067aa0ba902b7-01","com.example/revision":9007199254740993}`
	for round := 0; round < 2; round++ {
		continuation := ""
		if round == 1 {
			continuation = `,"requestState":"opaque state","inputResponses":{"approval":{"result":{"action":"accept"}}}`
		}
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":9007199254740993,"method":"tools/call","params":{"name":"fake-mcp.echo","arguments":{"revision":9007199254740993},%s%s}}`, meta, continuation)
		c, response := newMCPCallContext(t, fx.user.Id, fmt.Sprintf("mcp-upgrade-round-%d", round))
		c.Request.Body = io.NopCloser(strings.NewReader(body))
		c.Request.Header.Set("Authorization", "Bearer downstream-only-test-credential")
		c.Request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
		c.Request.Header.Set(mcp.MethodHeader, "tools/call")
		c.Request.Header.Set(mcp.NameHeader, "fake-mcp.echo")
		MCPProxyLatest(c)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), `"error":`)
		wire := <-observed
		require.Contains(t, wire, `"elicitation":{"url":{}}`)
		require.Contains(t, wire, `"com.example/revision":9007199254740993`)
		require.Contains(t, wire, `"revision":9007199254740993`)
		require.NotContains(t, wire, `"extensions"`, "do not negotiate unimplemented task lifecycle methods")
		require.Contains(t, response.Body.String(), `"id":9007199254740993`)
		if round == 0 {
			require.Contains(t, response.Body.String(), `"resultType":"input_required"`)
		} else {
			require.Contains(t, wire, `"requestState":"opaque state"`)
			require.Contains(t, wire, `"action":"accept"`)
			require.Contains(t, response.Body.String(), `"revision":9007199254740993`)
		}
		var count int64
		require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id LIKE ?", "mcp-upgrade-round-%").Count(&count).Error)
		require.Equal(t, int64(round), count, "input_required is not billed or audited as completed")
	}
	require.Equal(t, int64(2), upstreamCalls.Load())
}

// TestMCP20260929GatewayEmptyCatalog requires an empty array in tools/list rather than JSON null.
func TestMCP20260929GatewayEmptyCatalog(t *testing.T) {
	cleanup, fx := setupMCPProxyTest(t)
	defer cleanup()
	deleted := model.DB.Delete(&model.MCPTool{}, fx.tool.Id)
	require.NoError(t, deleted.Error)
	require.Equal(t, int64(1), deleted.RowsAffected)
	c, response := newMCPCallContext(t, fx.user.Id, "mcp-upgrade-empty")
	c.Request.Body = io.NopCloser(bytes.NewReader(modernMCPRequestBody(t, "empty", "tools/list", nil)))
	c.Request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	c.Request.Header.Set(mcp.MethodHeader, "tools/list")
	MCPProxyLatest(c)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"tools":[]`)
}
