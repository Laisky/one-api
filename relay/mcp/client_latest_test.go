package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestListToolsLatestUsesModernRequestModel verifies tool discovery no longer requires initialize on modern servers.
func TestListToolsLatestUsesModernRequestModel(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, ProtocolVersion, r.Header.Get(ProtocolVersionHeader))
		require.Equal(t, "tools/list", r.Header.Get(MethodHeader))
		require.Empty(t, r.Header.Get(SessionIDHeader))
		var request struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "tools/list", request.Method)
		meta := request.Params["_meta"].(map[string]any)
		require.Equal(t, ProtocolVersion, meta[MetaProtocolVersionKey])
		require.NotNil(t, meta[MetaClientCapabilitiesKey])
		w.Header().Set("Content-Type", "application/json")
		writeMCPTestJSON(t, w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"resultType": "complete",
				"tools": []any{map[string]any{
					"name":        "echo",
					"title":       "Echo",
					"description": "echo input",
					"inputSchema": map[string]any{"type": "object"},
				}},
			},
		})
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	tools, err := client.ListToolsLatest(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, requests)
	require.Equal(t, "Echo", tools[0].Title)
}

// TestCallToolLatestSendsSchemaDrivenHeaders verifies named and parameter headers mirror the JSON body.
func TestCallToolLatestSendsSchemaDrivenHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "tools/call", r.Header.Get(MethodHeader))
		require.Equal(t, "echo", r.Header.Get(NameHeader))
		require.Equal(t, "acme", r.Header.Get("Mcp-Param-Tenant"))
		var request struct {
			ID any `json:"id"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "application/json")
		writeMCPTestJSON(t, w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"resultType":        "complete",
				"content":           []any{map[string]any{"type": "text", "text": "ok"}},
				"structuredContent": map[string]any{"tenant": "acme"},
				"isError":           false,
			},
		})
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	result, err := client.CallToolLatestWithDescriptor(context.Background(), ToolDescriptor{
		Name: "echo",
		InputSchema: map[string]any{
			"properties": map[string]any{
				"tenant": map[string]any{"type": "string", "x-mcp-header": "Tenant"},
			},
		},
	}, map[string]any{"tenant": "acme"})
	require.NoError(t, err)
	require.Equal(t, ResultTypeComplete, result.ResultType)
	require.Equal(t, map[string]any{"tenant": "acme"}, result.StructuredContent)
}

// TestListToolsLatestFallsBackToLegacy verifies old Streamable HTTP servers retain initialize compatibility.
func TestListToolsLatestFallsBackToLegacy(t *testing.T) {
	var mu sync.Mutex
	methods := make([]string, 0, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		mu.Lock()
		methods = append(methods, request.Method)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if request.Method == "tools/list" && r.Header.Get(MethodHeader) != "" {
			w.WriteHeader(http.StatusBadRequest)
			writeMCPTestText(t, w, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"initialize required"}}`)
			return
		}
		switch request.Method {
		case "initialize":
			writeMCPTestJSON(t, w, map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"protocolVersion": LegacyProtocolVersionFallback,
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "legacy", "version": "1"},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeMCPTestJSON(t, w, map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result":  map[string]any{"tools": []any{map[string]any{"name": "legacy.echo"}}},
			})
		}
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	tools, err := client.ListToolsLatest(context.Background())
	require.NoError(t, err)
	require.Equal(t, "legacy.echo", tools[0].Name)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"tools/list", "initialize", "notifications/initialized", "tools/list"}, methods)
}

// TestCallToolResultAcceptsModernAndLegacyAliases verifies the wire model preserves both protocol eras.
func TestCallToolResultAcceptsModernAndLegacyAliases(t *testing.T) {
	var modern CallToolResult
	require.NoError(t, json.Unmarshal([]byte(`{"resultType":"input_required","inputRequests":{"confirm":{"method":"elicitation/create"}},"requestState":"opaque"}`), &modern))
	require.Equal(t, ResultTypeInputRequired, modern.ResultType)
	require.Equal(t, map[string]any{"method": "elicitation/create"}, modern.InputRequests["confirm"])
	require.Equal(t, "opaque", modern.RequestState)

	var legacy CallToolResult
	require.NoError(t, json.Unmarshal([]byte(`{"is_error":true,"structured_content":{"step":2}}`), &legacy))
	require.Empty(t, legacy.ResultType)
	require.True(t, legacy.IsError)
}

// TestCallToolLatestEncodesNameAndMRTRFields verifies non-ASCII tool names and retry state follow the modern wire format.
func TestCallToolLatestEncodesNameAndMRTRFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "=?base64?5aSp5rCU?=", r.Header.Get(NameHeader))
		var request struct {
			ID     any            `json:"id"`
			Params map[string]any `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "state", request.Params["requestState"])
		require.Equal(t, map[string]any{"answer": "yes"}, request.Params["inputResponses"])
		w.Header().Set("Content-Type", "application/json")
		writeMCPTestJSON(t, w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"resultType": "complete",
				"content":    []any{},
			},
		})
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	_, err := client.CallToolLatestWithOptions(context.Background(), ToolDescriptor{
		Name:        "天气",
		InputSchema: map[string]any{"type": "object"},
	}, map[string]any{}, CallToolRequestOptions{
		InputResponses: map[string]any{"answer": "yes"},
		RequestState:   "state",
	})
	require.NoError(t, err)
}

// TestListToolsLatestExcludesInvalidHeaderSchemas verifies malformed tools are filtered instead of failing the entire catalog.
func TestListToolsLatestExcludesInvalidHeaderSchemas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID any `json:"id"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "application/json")
		writeMCPTestJSON(t, w, map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result": map[string]any{
				"resultType": "complete",
				"tools": []any{
					map[string]any{"name": "valid", "inputSchema": map[string]any{"type": "object"}},
					map[string]any{"name": "invalid", "inputSchema": map[string]any{"items": map[string]any{"x-mcp-header": "Bad"}}},
				},
			},
		})
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	tools, err := client.ListToolsLatest(context.Background())
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "valid", tools[0].Name)
}

// TestRecognizedModernErrorsDoNotFallBack verifies validation errors are not misclassified as legacy servers.
func TestRecognizedModernErrorsDoNotFallBack(t *testing.T) {
	for _, code := range []int{ErrorCodeHeaderMismatch, ErrorCodeMissingRequiredClientCapability, ErrorCodeUnsupportedProtocolVersion} {
		require.False(t, IsModernFallbackCandidate(&ProtocolError{HTTPStatus: http.StatusBadRequest, Code: code}))
	}
	require.False(t, IsModernFallbackCandidate(&ProtocolError{HTTPStatus: http.StatusNotFound, Code: -32601}))
	require.True(t, IsModernFallbackCandidate(&ProtocolError{HTTPStatus: http.StatusBadRequest, Code: -32600, Message: "initialize required"}))
}

// TestListToolsLatestFollowsPagination verifies modern tool catalogs are collected across cursors.
func TestListToolsLatestFollowsPagination(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			ID     any            `json:"id"`
			Params map[string]any `json:"params"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		result := map[string]any{
			"resultType": "complete",
			"tools": []any{map[string]any{
				"name":        fmt.Sprintf("tool-%d", calls),
				"inputSchema": map[string]any{"type": "object"},
			}},
			"ttlMs":      1000,
			"cacheScope": "private",
		}
		if calls == 1 {
			require.Empty(t, request.Params["cursor"])
			result["nextCursor"] = "page-2"
		} else {
			require.Equal(t, "page-2", request.Params["cursor"])
		}
		w.Header().Set("Content-Type", "application/json")
		writeMCPTestJSON(t, w, map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	tools, err := client.ListToolsLatest(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"tool-1", "tool-2"}, []string{tools[0].Name, tools[1].Name})
	require.Equal(t, 2, calls)
}

// writeMCPTestJSON encodes one mock HTTP response and reports failures at the write site.
//
// Parameters:
//   - t: The owning test receives any encoding failure.
//   - writer: The mock HTTP response writer receives the JSON payload.
//   - value: The JSON-compatible response value is encoded.
//
// Return values: none; failures are reported through t.
func writeMCPTestJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	require.NoError(t, json.NewEncoder(writer).Encode(value))
}

// writeMCPTestText writes one mock HTTP response and reports failures at the write site.
//
// Parameters:
//   - t: The owning test receives any write failure.
//   - writer: The mock HTTP response writer receives the text payload.
//   - value: The response text is written exactly once.
//
// Return values: none; failures are reported through t.
func writeMCPTestText(t *testing.T, writer http.ResponseWriter, value string) {
	t.Helper()
	_, err := fmt.Fprint(writer, value)
	require.NoError(t, err)
}

// TestUnsupportedProtocolVersionHonoursAdvertisedVersions pins that a modern
// server which only speaks a legacy revision is still usable.
//
// The 2026-07-28 binding says that on a recognized modern error the client should
// "retry using the advertised `supported` versions ... rather than falling back".
// -32022 carries data.supported for exactly that purpose, but the list was parsed
// into ProtocolError.Data and then thrown away: IsRecognizedModernError returned
// true, IsModernFallbackCandidate returned false, and the client refused to talk
// to a server it fully supports through Initialize. A very likely transitional
// posture — 2026 header validation, 2025-11-25 as the only supported revision —
// was therefore unreachable.
func TestUnsupportedProtocolVersionHonoursAdvertisedVersions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		data         any
		wantFallback bool
	}{
		{
			name:         "advertises a legacy version we speak",
			data:         map[string]any{"supported": []any{LegacyProtocolVersion}},
			wantFallback: true,
		},
		{
			name:         "advertises the older legacy fallback",
			data:         map[string]any{"supported": []any{LegacyProtocolVersionFallback}},
			wantFallback: true,
		},
		{
			name:         "advertises only versions we cannot speak",
			data:         map[string]any{"supported": []any{"1999-01-01"}},
			wantFallback: false,
		},
		{
			name:         "advertises nothing",
			data:         nil,
			wantFallback: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &ProtocolError{
				HTTPStatus: http.StatusBadRequest,
				Code:       ErrorCodeUnsupportedProtocolVersion,
				Message:    "unsupported protocol version",
				Data:       tc.data,
			}
			require.Equal(t, tc.wantFallback, IsModernFallbackCandidate(err))
		})
	}
}
