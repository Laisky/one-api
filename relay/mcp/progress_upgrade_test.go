package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/model"
	"github.com/stretchr/testify/require"
)

// TestMCPProgressStreaming verifies live request-correlated progress, compact/multiline events and terminal delivery.
func TestMCPProgressStreaming(t *testing.T) {
	for _, separator := range []string{"\n", "\r\n", "\r"} {
		t.Run(fmt.Sprintf("newline-%q", separator), func(t *testing.T) {
			observed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID string `json:"id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				stream := ": comment\n\n" +
					"data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progressToken\":9007199254740992,\"progress\":1}}\n\n" +
					"data: {\"jsonrpc\":\"2.0\",\n" +
					"data: \"method\":\"notifications/progress\",\"params\":{\"progressToken\":9007199254740993,\"progress\":2}}\n\n"
				if _, err := io.WriteString(w, strings.ReplaceAll(stream, "\n", separator)); err != nil {
					t.Error(err)
					return
				}
				w.(http.Flusher).Flush()
				select {
				case <-observed:
				case <-r.Context().Done():
					return
				}
				if _, err := fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"resultType\":\"complete\",\"content\":[]}}%s%s", request.ID, separator, separator); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, time.Second)
			count := 0
			_, err := client.CallToolLatestWithOptions(context.Background(), ToolDescriptor{Name: "echo"}, nil, CallToolRequestOptions{
				Meta: map[string]any{"progressToken": json.Number("9007199254740993")},
				OnNotification: func(_ context.Context, payload json.RawMessage) error {
					count++
					require.Contains(t, string(payload), `"progressToken":9007199254740993`)
					if count == 1 {
						close(observed)
					}
					return nil
				},
			})
			require.NoError(t, err)
			require.Equal(t, 1, count)
		})
	}
}

// TestMCPProgressFailurePreventsReplay proves a failed progress sink stops both protocol and candidate retries.
func TestMCPProgressFailurePreventsReplay(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if _, err := io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progressToken\":\"one\",\"progress\":1}}\n\n"); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, time.Second)
	failure := errors.New("downstream disconnected")
	_, _, err := CallWithFallback(context.Background(), []ToolCandidate{{ServerID: 1}, {ServerID: 2}}, func(ctx context.Context, _ ToolCandidate) (*CallToolResult, error) {
		return client.CallToolLatestWithOptions(ctx, ToolDescriptor{Name: "echo"}, nil, CallToolRequestOptions{
			Meta:           map[string]any{"progressToken": "one"},
			OnNotification: func(context.Context, json.RawMessage) error { return failure },
		})
	})
	require.ErrorIs(t, err, failure)
	require.Equal(t, int64(1), hits.Load())
}

// TestMCPCallMetadataIsolation exercises one shared client with concurrent calls having distinct capabilities and trace state.
func TestMCPCallMetadataIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     string `json:"id"`
			Params struct {
				Arguments map[string]any `json:"arguments"`
				Meta      map[string]any `json:"_meta"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Params.Arguments["marker"] != request.Params.Meta["com.example/marker"] {
			t.Error("metadata crossed concurrent call boundaries")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []any{}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	var group sync.WaitGroup
	failures := make(chan error, 32)
	for index := range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			marker := fmt.Sprintf("caller-%d", index)
			_, err := client.CallToolLatestWithOptions(context.Background(), ToolDescriptor{Name: "echo"}, map[string]any{"marker": marker}, CallToolRequestOptions{Meta: map[string]any{"com.example/marker": marker}})
			failures <- err
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
}

// TestMCPExactNumberBoundaries verifies integer detection does not round fractional values or expand hostile exponents.
func TestMCPExactNumberBoundaries(t *testing.T) {
	for _, value := range []string{"9007199254740993", "18446744073709551615", "1.0", "10e-1", "1e999999999999999999999999", "0e-999999999999999999999999"} {
		require.True(t, IsJSONInteger(json.Number(value)), value)
	}
	for _, value := range []string{"9007199254740993.5", "1.0000000000000001", "1e-999999999999999999999999", "1e-9223372036854775808", "true", "null", `"1"`} {
		require.False(t, IsJSONInteger(json.Number(value)), value)
	}
	_, err := renderInteger(json.Number("1e999999999999999999999999"))
	require.Error(t, err)
	var result map[string]any
	require.Error(t, DecodeJSON([]byte(`{} {}`), &result))
}

// TestMCPUnrequestedElicitationRejected verifies URL-only workflows cannot silently run on form-only clients.
func TestMCPUnrequestedElicitationRejected(t *testing.T) {
	result := &CallToolResult{ResultType: ResultTypeInputRequired, InputRequests: map[string]any{"approve": map[string]any{"method": "elicitation/create", "params": map[string]any{"mode": "url"}}}}
	require.Error(t, validateMCPToolInputCapabilities(result, nil))
	require.Error(t, validateMCPToolInputCapabilities(result, map[string]any{MetaClientCapabilitiesKey: map[string]any{"elicitation": map[string]any{}}}))
	require.NoError(t, validateMCPToolInputCapabilities(result, map[string]any{MetaClientCapabilitiesKey: map[string]any{"elicitation": map[string]any{"url": map[string]any{}}}}))
}
