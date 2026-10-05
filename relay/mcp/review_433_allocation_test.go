package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestMCP433ReviewMetadataCopy verifies nil, empty, populated and large parameter copies on the HTTP wire.
// These are equivalence controls for CodeQL #177, not a fabricated reproduction of an impossible-sized map.
func TestMCP433ReviewMetadataCopy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     string         `json:"id"`
			Params map[string]any `json:"params"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&request); err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": request.Params}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client := NewStreamableHTTPClient(&model.MCPServer{BaseURL: server.URL}, nil, 5*time.Second)
	for _, count := range []int{-1, 0, 1, 4096} {
		for _, override := range []bool{false, true} {
			t.Run(fmt.Sprintf("entries-%d/override-%t", count, override), func(t *testing.T) {
				var params map[string]any
				if count >= 0 {
					params = make(map[string]any)
					for index := 0; index < count; index++ {
						params[fmt.Sprintf("field-%d", index)] = json.Number("9007199254740993")
					}
					params["_meta"] = map[string]any{"com.example/source": "params"}
				}
				options := CallToolRequestOptions{}
				if override {
					options.Meta = map[string]any{"com.example/source": "options", MetaProtocolVersionKey: "caller-version"}
				}
				beforeParams, err := json.Marshal(params)
				require.NoError(t, err)
				beforeMeta, err := json.Marshal(options.Meta)
				require.NoError(t, err)
				var result map[string]any
				require.NoError(t, client.doModernRPCWithOptions(context.Background(), "tools/call", params, "echo", nil, &result, options))
				for index := 0; index < count; index++ {
					require.Equal(t, json.Number("9007199254740993"), result[fmt.Sprintf("field-%d", index)])
				}
				meta, ok := result["_meta"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, ProtocolVersion, meta[MetaProtocolVersionKey])
				require.Equal(t, map[string]any{}, meta[MetaClientCapabilitiesKey])
				if override {
					require.Equal(t, "options", meta["com.example/source"])
				} else if count >= 0 {
					require.Equal(t, "params", meta["com.example/source"])
				} else {
					require.NotContains(t, meta, "com.example/source")
				}
				afterParams, err := json.Marshal(params)
				require.NoError(t, err)
				afterMeta, err := json.Marshal(options.Meta)
				require.NoError(t, err)
				require.Equal(t, beforeParams, afterParams)
				require.Equal(t, beforeMeta, afterMeta)
			})
		}
	}
}

// TestMCP433ReviewExtensionCopies covers adjacent descriptor and result allocation sites with wire round trips.
// Unknown fields and large integer values survive, while authoritative typed fields override collisions.
func TestMCP433ReviewExtensionCopies(t *testing.T) {
	for _, count := range []int{-1, 0, 1, 4096} {
		t.Run(fmt.Sprintf("entries-%d", count), func(t *testing.T) {
			var extensions map[string]any
			if count >= 0 {
				extensions = make(map[string]any)
				for index := 0; index < count; index++ {
					extensions[fmt.Sprintf("com.example/field-%d", index)] = json.Number("18446744073709551615")
				}
				extensions["name"] = "untrusted-name"
				extensions["resultType"] = "untrusted-type"
			}
			before, err := json.Marshal(extensions)
			require.NoError(t, err)
			for _, value := range []any{
				ToolDescriptor{Name: "echo", AdditionalFields: extensions},
				CallToolResult{ResultType: ResultTypeComplete, AdditionalFields: extensions},
			} {
				encoded, err := json.Marshal(value)
				require.NoError(t, err)
				var decoded map[string]any
				require.NoError(t, DecodeJSON(encoded, &decoded))
				for index := 0; index < count; index++ {
					require.Equal(t, json.Number("18446744073709551615"), decoded[fmt.Sprintf("com.example/field-%d", index)])
				}
				switch value.(type) {
				case ToolDescriptor:
					require.Equal(t, "echo", decoded["name"])
				case CallToolResult:
					require.Equal(t, ResultTypeComplete, decoded["resultType"])
				}
			}
			after, err := json.Marshal(extensions)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
