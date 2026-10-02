package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// newMCPProgressTestGateway creates a real HTTP gateway with the fixture's authenticated identity and request logger.
// Its cleanup closes the HTTP listener before fixture database cleanup occurs.
func newMCPProgressTestGateway(t *testing.T, userID int, requestID string) *httptest.Server {
	t.Helper()
	router := gin.New()
	router.POST("/mcp", func(c *gin.Context) {
		c.Set(ctxkey.Id, userID)
		c.Set(ctxkey.RequestId, requestID)
		c.Set(helper.RequestIdKey, requestID)
		gmw.SetLogger(c, logger.Logger)
		MCPProxyLatest(c)
	})
	return httptest.NewServer(router)
}

// TestMCPGatewayLiveProgress proves the gateway flushes progress before the upstream can produce its final result.
func TestMCPGatewayLiveProgress(t *testing.T) {
	cleanup, fixture := setupMCPProxyTest(t)
	defer cleanup()
	observed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progressToken\":\"live-progress\",\"progress\":1}}\n\n"); err != nil {
			t.Error(err)
			return
		}
		w.(http.Flusher).Flush()
		select {
		case <-observed:
		case <-r.Context().Done():
			return
		}
		if _, err := fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"resultType\":\"complete\",\"structuredContent\":null}}\n\n", request.ID); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()
	require.NoError(t, model.DB.Model(&model.MCPServer{}).Where("id = ?", fixture.server.Id).Update("base_url", upstream.URL).Error)
	gateway := newMCPProgressTestGateway(t, fixture.user.Id, "mcp-live-progress")
	defer gateway.Close()
	client := mcp.NewStreamableHTTPClient(&model.MCPServer{BaseURL: gateway.URL + "/mcp"}, nil, 2*time.Second)
	count := 0
	result, err := client.CallToolLatestWithOptions(context.Background(), mcp.ToolDescriptor{Name: "fake-mcp.echo"}, nil, mcp.CallToolRequestOptions{
		Meta: map[string]any{"progressToken": "live-progress"},
		OnNotification: func(_ context.Context, notification json.RawMessage) error {
			count++
			require.Contains(t, string(notification), `"progressToken":"live-progress"`)
			if count == 1 {
				close(observed)
			}
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"structuredContent":null`)
	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", "mcp-live-progress").Count(&logs).Error)
	require.Equal(t, int64(1), logs)
}

// TestMCPGatewayStreamCancellation verifies closing downstream work cancels upstream I/O and does not bill completion.
func TestMCPGatewayStreamCancellation(t *testing.T) {
	cleanup, fixture := setupMCPProxyTest(t)
	defer cleanup()
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progressToken\":\"cancel-me\",\"progress\":1}}\n\n"); err != nil {
			t.Error(err)
			return
		}
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-release:
		}
	}))
	defer upstream.Close()
	defer close(release)
	require.NoError(t, model.DB.Model(&model.MCPServer{}).Where("id = ?", fixture.server.Id).Update("base_url", upstream.URL).Error)
	gateway := newMCPProgressTestGateway(t, fixture.user.Id, "mcp-cancelled-progress")
	defer gateway.Close()
	client := mcp.NewStreamableHTTPClient(&model.MCPServer{BaseURL: gateway.URL + "/mcp"}, nil, 3*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := client.CallToolLatestWithOptions(ctx, mcp.ToolDescriptor{Name: "fake-mcp.echo"}, nil, mcp.CallToolRequestOptions{
		Meta:           map[string]any{"progressToken": "cancel-me"},
		OnNotification: func(context.Context, json.RawMessage) error { cancel(); return context.Canceled },
	})
	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("downstream cancellation did not reach upstream")
	}
	require.Equal(t, int64(1), calls.Load())
	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", "mcp-cancelled-progress").Count(&logs).Error)
	require.Zero(t, logs)
}

// TestMCPGatewaySSEFailureEnvelope verifies a truncated upstream stream ends with an SSE error, never a mixed JSON response.
func TestMCPGatewaySSEFailureEnvelope(t *testing.T) {
	cleanup, fixture := setupMCPProxyTest(t)
	defer cleanup()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progressToken\":\"once\",\"progress\":1}}\n\n"); err != nil {
			t.Error(err)
		}
	}))
	defer upstream.Close()
	require.NoError(t, model.DB.Model(&model.MCPServer{}).Where("id = ?", fixture.server.Id).Update("base_url", upstream.URL).Error)
	c, response := newMCPCallContext(t, fixture.user.Id, "mcp-truncated-progress")
	params := mcp.WithModernMeta(map[string]any{"name": "fake-mcp.echo", "arguments": map[string]any{}, "_meta": map[string]any{"progressToken": "once"}})
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "truncated", "method": "tools/call", "params": params})
	require.NoError(t, err)
	c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
	c.Request.Header.Set("Accept", "application/json, text/event-stream")
	c.Request.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	c.Request.Header.Set(mcp.MethodHeader, "tools/call")
	c.Request.Header.Set(mcp.NameHeader, "fake-mcp.echo")
	MCPProxyLatest(c)
	require.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
	require.Equal(t, "no", response.Header().Get("X-Accel-Buffering"))
	require.Equal(t, 2, strings.Count(response.Body.String(), "event: message\ndata: "))
	require.Contains(t, response.Body.String(), `"code":-32603`)
	require.Contains(t, response.Body.String(), `"id":"truncated"`)
}

// TestMCPGatewayJSONOnlyNegotiation verifies quality-zero and JSON-only Accept headers cannot start SSE responses.
func TestMCPGatewayJSONOnlyNegotiation(t *testing.T) {
	for _, accept := range []string{"", "application/json", "text/event-stream;q=0", "text/event-stream;q=NaN", "text/event-stream;q=2"} {
		require.False(t, acceptsModernMCPSSE(http.Header{"Accept": []string{accept}}), accept)
	}
	require.True(t, acceptsModernMCPSSE(http.Header{"Accept": []string{"application/json, text/event-stream;q=0.5"}}))
}
