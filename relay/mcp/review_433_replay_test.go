package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestMCP433ReviewLegacyReplay exercises real legacy executions, not synthetic retry errors.
// All entry points must stop candidate fallback after the first server may have performed a side effect.
func TestMCP433ReviewLegacyReplay(t *testing.T) {
	for _, entry := range []string{"explicit", "negotiated", "modern-fallback"} {
		for _, failure := range []string{"http-503", "truncated-json", "wrong-response-id"} {
			t.Run(entry+"/"+failure, func(t *testing.T) {
				var executions, attempts atomic.Int64
				server := newMCP433ReviewLegacyPeer(t, &executions, failure, false)
				defer server.Close()
				client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
				healthy := newMCP433ReviewLegacyPeer(t, &executions, "success", false)
				defer healthy.Close()
				other := NewStreamableHTTPClient(&model.MCPServer{BaseURL: healthy.URL}, nil, 5*time.Second)
				if entry == "negotiated" {
					require.NoError(t, client.Initialize(context.Background()))
				}
				_, result, err := CallWithFallback(context.Background(), []ToolCandidate{
					{ResolvedTool: ResolvedTool{ServerID: 1}},
					{ResolvedTool: ResolvedTool{ServerID: 2}},
				}, func(ctx context.Context, candidate ToolCandidate) (*CallToolResult, error) {
					attempts.Add(1)
					if candidate.ServerID == 2 {
						return other.CallTool(ctx, "change_state", map[string]any{"value": "new"})
					}
					if entry == "explicit" {
						return client.CallTool(ctx, "change_state", map[string]any{"value": "new"})
					}
					return client.CallToolLatest(ctx, "change_state", map[string]any{"value": "new"})
				})
				require.Equal(t, int64(1), executions.Load(), "an uncertain execution must not be repeated on another candidate")
				require.Equal(t, int64(1), attempts.Load())
				require.Nil(t, result)
				var uncertain *ToolExecutionUncertainError
				require.ErrorAs(t, err, &uncertain)
				if failure == "http-503" {
					var protocolErr *ProtocolError
					require.ErrorAs(t, err, &protocolErr, "the retry guard must preserve the original protocol error")
					require.Equal(t, http.StatusServiceUnavailable, protocolErr.HTTPStatus)
				}
			})
		}
	}
}

// TestMCP433ReviewLegacyControls preserves successful negotiation, tool errors, and safe pre-execution fallback.
// A failed initialize has not run a tool, and a valid isError result is not a reason to replay an operation.
func TestMCP433ReviewLegacyControls(t *testing.T) {
	for _, mode := range []string{"success", "tool-error", "initialize-failure", "invalid-arguments", "session-expired"} {
		t.Run(mode, func(t *testing.T) {
			var executions, attempts atomic.Int64
			server := newMCP433ReviewLegacyPeer(t, &executions, mode, mode == "initialize-failure")
			defer server.Close()
			client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
			healthy := newMCP433ReviewLegacyPeer(t, &executions, "success", false)
			defer healthy.Close()
			other := NewStreamableHTTPClient(&model.MCPServer{BaseURL: healthy.URL}, nil, 5*time.Second)
			selected, result, err := CallWithFallback(context.Background(), []ToolCandidate{
				{ResolvedTool: ResolvedTool{ServerID: 1}},
				{ResolvedTool: ResolvedTool{ServerID: 2}},
			}, func(ctx context.Context, candidate ToolCandidate) (*CallToolResult, error) {
				attempts.Add(1)
				if candidate.ServerID == 2 {
					return other.CallTool(ctx, "change_state", nil)
				}
				if mode == "invalid-arguments" {
					return client.CallTool(ctx, "change_state", []string{"not-an-object"})
				}
				return client.CallToolLatest(ctx, "change_state", nil)
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, mode == "tool-error", result.IsError)
			require.Equal(t, int64(1), executions.Load())
			if mode == "initialize-failure" || mode == "invalid-arguments" {
				require.Equal(t, 2, selected.ServerID)
				require.Equal(t, int64(2), attempts.Load())
			} else {
				require.Equal(t, 1, selected.ServerID)
				require.Equal(t, int64(1), attempts.Load())
			}
		})
	}
}

// newMCP433ReviewLegacyPeer returns an HTTP fixture which counts actual tools/call executions.
// It rejects modern traffic before execution, supports initialization and notification, and injects a chosen receipt fault.
func newMCP433ReviewLegacyPeer(t *testing.T, executions *atomic.Int64, mode string, failInitialize bool) *httptest.Server {
	t.Helper()
	var expired atomic.Bool
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     string `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		write := func(status int, body any) {
			w.WriteHeader(status)
			if err := json.NewEncoder(w).Encode(body); err != nil {
				t.Error(err)
			}
		}
		rpcError := func(status, code int, message string) {
			write(status, map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": code, "message": message}})
		}
		if r.Header.Get(ProtocolVersionHeader) == ProtocolVersion {
			rpcError(http.StatusBadRequest, -32600, "initialize required")
			return
		}
		switch request.Method {
		case "initialize":
			if failInitialize {
				rpcError(http.StatusServiceUnavailable, -32603, "initialize unavailable")
				return
			}
			w.Header().Set(SessionIDHeader, "review-session")
			write(http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{
				"protocolVersion": LegacyProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{"name": "review-peer", "version": "1"},
			}})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			if mode == "session-expired" && !expired.Swap(true) {
				rpcError(http.StatusNotFound, -32000, "session expired before execution")
				return
			}
			executions.Add(1)
			switch mode {
			case "http-503":
				rpcError(http.StatusServiceUnavailable, -32603, "receipt unavailable after execution")
			case "truncated-json":
				if _, err := fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":`, request.ID); err != nil {
					t.Error(err)
				}
			case "wrong-response-id":
				write(http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": "another-call", "result": map[string]any{"content": []any{}}})
			default:
				write(http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []any{}, "isError": mode == "tool-error"}})
			}
		default:
			rpcError(http.StatusBadRequest, -32601, "unexpected method")
		}
	}))
}
