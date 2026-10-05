package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestMCP20260929RequestMetadata verifies caller capabilities and opaque metadata survive without mutation.
func TestMCP20260929RequestMetadata(t *testing.T) {
	meta := map[string]any{
		MetaClientCapabilitiesKey: map[string]any{"elicitation": map[string]any{"url": map[string]any{}}},
		"progressToken":           "request-only",
		"com.example/revision":    json.Number("9007199254740993"),
	}
	params := map[string]any{"name": "echo", "_meta": meta}
	before, err := json.Marshal(params)
	require.NoError(t, err)
	encoded, err := json.Marshal(WithModernMeta(params))
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"elicitation":{"url":{}}`)
	require.Contains(t, string(encoded), `"progressToken":"request-only"`)
	require.Contains(t, string(encoded), `"com.example/revision":9007199254740993`)
	after, err := json.Marshal(params)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "metadata belongs to the caller")
}

// TestMCP20260929SSETerminalBeforeEOF proves a flushed final response does not wait for remote connection closure.
func TestMCP20260929SSETerminalBeforeEOF(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := fmt.Fprintf(w, ": heartbeat\r\nevent: message\r\ndata: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"resultType\":\"complete\",\"content\":[]}}\r\n\r\n", request.ID); err != nil {
			t.Error(err)
			return
		}
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 750*time.Millisecond)
	result, err := client.CallToolLatest(context.Background(), "echo", nil)
	require.NoError(t, err, "a final SSE result completes the call even while the server keeps its stream open")
	require.Equal(t, ResultTypeComplete, result.ResultType)
}

// TestMCP20260929ResultEnvelope verifies malformed modern results cannot become successful tool executions.
func TestMCP20260929ResultEnvelope(t *testing.T) {
	for _, fields := range []string{
		`"result":null`,
		`"result":[]`,
		`"result":{ "resultType":"task" }`,
		`"result":{ "resultType":null }`,
		`"result":{ "resultType":"" }`,
		`"result":{ "resultType":"input_required" }`,
		`"result":{},"error":{"code":-32603,"message":"failed"}`,
		`"result":{},"error":null`,
	} {
		t.Run(fields, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID string `json:"id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,%s}`, request.ID, fields); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, time.Second)
			_, err := client.CallToolLatest(context.Background(), "echo", nil)
			require.Error(t, err)
		})
	}
}

// TestMCP20260929OpaqueJSON verifies large integers and explicit structured null survive typed round trips.
func TestMCP20260929OpaqueJSON(t *testing.T) {
	for _, payload := range []string{
		`{"resultType":"complete","structuredContent":{"revision":9007199254740993},"com.example/value":18446744073709551615,"_meta":{"revision":9007199254740993}}`,
		`{"resultType":"complete","structuredContent":null}`,
	} {
		t.Run(payload, func(t *testing.T) {
			var result CallToolResult
			require.NoError(t, json.Unmarshal([]byte(payload), &result))
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			require.Equal(t, compactMCPUpgradeJSON(t, payload), compactMCPUpgradeJSON(t, string(encoded)))
		})
	}
	var descriptor ToolDescriptor
	require.NoError(t, json.Unmarshal([]byte(`{"name":"echo","inputSchema":{"const":9007199254740993},"com.example/limit":18446744073709551615}`), &descriptor))
	encoded, err := json.Marshal(descriptor)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `9007199254740993`)
	require.Contains(t, string(encoded), `18446744073709551615`)
	arguments, err := normalizeToolArguments(struct {
		Revision uint64 `json:"revision"`
	}{9007199254740993})
	require.NoError(t, err)
	encoded, err = json.Marshal(arguments)
	require.NoError(t, err)
	require.Equal(t, `{"revision":9007199254740993}`, string(encoded))
}

// compactMCPUpgradeJSON canonicalizes JSON with exact number tokens for comparisons that cannot use float64.
func compactMCPUpgradeJSON(t *testing.T, raw string) string {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	require.NoError(t, decoder.Decode(&value))
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}
