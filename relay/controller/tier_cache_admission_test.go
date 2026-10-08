package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestTierAdmissionCacheWritesStructural verifies parsed request cache markers
// select only their declared ephemeral write buckets without reading user data.
func TestTierAdmissionCacheWritesStructural(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		request any
		write5m bool
		write1h bool
	}{
		{
			name:    "no cache marker",
			request: map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}},
		},
		{
			name: "default ephemeral system",
			request: map[string]any{"system": []any{map[string]any{
				"type": "text", "text": "system", "cache_control": map[string]any{"type": "ephemeral"},
			}}},
			write5m: true,
		},
		{
			name:    "automatic one hour root",
			request: map[string]any{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}, "messages": []any{map[string]any{"role": "user", "content": "hello"}}},
			write1h: true,
		},
		{
			name:    "automatic default root",
			request: map[string]any{"cache_control": map[string]any{"type": "ephemeral"}},
			write5m: true,
		},
		{
			name: "explicit five minute tool",
			request: map[string]any{"tools": []any{map[string]any{
				"name": "search", "cache_control": map[string]any{"type": "ephemeral", "ttl": "5m"},
			}}},
			write5m: true,
		},
		{
			name: "one hour message block",
			request: map[string]any{"messages": []any{map[string]any{
				"role": "user", "content": []any{map[string]any{
					"type": "text", "text": "hello", "cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"},
				}},
			}}},
			write1h: true,
		},
		{
			name: "mixed marker buckets",
			request: map[string]any{
				"system": []any{map[string]any{"cache_control": map[string]any{"type": "ephemeral"}}},
				"tools":  []any{map[string]any{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}}},
			},
			write5m: true, write1h: true,
		},
		{
			name: "metadata and transport extras excluded",
			request: map[string]any{
				"metadata":   map[string]any{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}},
				"extra_body": map[string]any{"cache_control": map[string]any{"type": "ephemeral"}},
				"messages": []any{map[string]any{
					"metadata":   map[string]any{"cache_control": map[string]any{"type": "ephemeral"}},
					"extra_body": map[string]any{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}},
				}},
			},
		},
		{
			name: "schema and tool input excluded",
			request: map[string]any{
				"tools": []any{map[string]any{
					"input_schema": map[string]any{"examples": []any{map[string]any{
						"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"},
					}}},
					"schema": map[string]any{"cache_control": map[string]any{"type": "ephemeral"}},
				}},
				"messages": []any{map[string]any{"content": []any{map[string]any{
					"type":      "tool_use",
					"input":     map[string]any{"cache_control": map[string]any{"type": "ephemeral"}},
					"arguments": map[string]any{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}},
					"data":      map[string]any{"cache_control": map[string]any{"type": "ephemeral"}},
				}}}},
			},
		},
		{
			name: "text containing JSON is not a marker",
			request: map[string]any{
				"system": "{\"cache_control\":{\"type\":\"ephemeral\",\"ttl\":\"1h\"}}",
				"messages": []any{map[string]any{"content": []any{map[string]any{
					"type": "text", "text": "{\"cache_control\":{\"type\":\"ephemeral\"}}",
				}}}},
			},
		},
		{
			name: "unknown control type does not infer a write",
			request: map[string]any{"tools": []any{map[string]any{
				"cache_control": map[string]any{"type": "other", "ttl": "1h"},
			}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			write5m, write1h, err := tierAdmissionCacheWrites(tc.request)
			require.NoError(t, err)
			require.Equal(t, tc.write5m, write5m)
			require.Equal(t, tc.write1h, write1h)
		})
	}
}

// TestTierAdmissionCacheWritesMalformed verifies invalid cache controls and
// unserializable canonical payloads return errors instead of a partial quote.
func TestTierAdmissionCacheWritesMalformed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		request any
		error   string
	}{
		{
			name:    "nonobject control",
			request: map[string]any{"system": []any{map[string]any{"cache_control": "ephemeral"}}},
			error:   "cache_control must be an object",
		},
		{
			name: "unsupported ttl",
			request: map[string]any{"tools": []any{map[string]any{
				"cache_control": map[string]any{"type": "ephemeral", "ttl": "2h"},
			}}},
			error: "unsupported ttl",
		},
		{
			name: "nonstring ttl",
			request: map[string]any{"messages": []any{map[string]any{
				"cache_control": map[string]any{"type": "ephemeral", "ttl": 3600},
			}}},
			error: "unsupported ttl",
		},
		{name: "nil request", request: nil, error: "must be an object"},
		{name: "nonobject root", request: []any{}, error: "decode tier admission cache request"},
		{name: "unserializable payload", request: map[string]any{"messages": func() {}}, error: "encode tier admission cache request"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			write5m, write1h, err := tierAdmissionCacheWrites(tc.request)
			require.ErrorContains(t, err, tc.error)
			require.False(t, write5m)
			require.False(t, write1h)
		})
	}
}

// TestTierAdmissionCacheWritesBoundedDepth verifies structural cache traversal
// rejects excessive nesting rather than recursively walking unbounded input.
func TestTierAdmissionCacheWritesBoundedDepth(t *testing.T) {
	t.Parallel()
	var content any = map[string]any{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}}
	for range tierAdmissionCacheMaxDepth + 1 {
		content = []any{content}
	}
	write5m, write1h, err := tierAdmissionCacheWrites(map[string]any{"messages": content})
	require.ErrorContains(t, err, "nesting limit")
	require.False(t, write5m)
	require.False(t, write1h)
}

// TestTierAdmissionPreparedPayload verifies exact provider bytes take precedence
// over converted and canonical requests so admission follows the dispatched
// cache controls without re-reading the upstream body.
func TestTierAdmissionPreparedPayload(t *testing.T) {
	t.Parallel()
	fallback := map[string]any{"cache_control": map[string]any{"type": "ephemeral"}}
	converted := map[string]any{"cache_control": map[string]any{"type": "ephemeral", "ttl": "1h"}}
	t.Run("nil context uses fallback", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, fallback, tierAdmissionPreparedPayload(nil, fallback))
	})
	t.Run("missing preparation uses fallback", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		require.Equal(t, fallback, tierAdmissionPreparedPayload(c, fallback))
	})
	t.Run("converted payload replaces fallback", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(ctxkey.ConvertedRequest, converted)
		payload := tierAdmissionPreparedPayload(c, fallback)
		write5m, write1h, err := tierAdmissionCacheWrites(payload)
		require.NoError(t, err)
		require.False(t, write5m)
		require.True(t, write1h)
	})
	t.Run("exact provider bytes replace converted markers", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		body := []byte(`{"tools":[{"name":"search","cache_control":{"type":"ephemeral","ttl":"5m"}}]}`)
		c.Set(tierAdmissionProviderBodyKey, body)
		c.Set(ctxkey.ConvertedRequest, converted)
		payload := tierAdmissionPreparedPayload(c, fallback)
		require.IsType(t, json.RawMessage{}, payload)
		require.Equal(t, body, []byte(payload.(json.RawMessage)))
		write5m, write1h, err := tierAdmissionCacheWrites(payload)
		require.NoError(t, err)
		require.True(t, write5m)
		require.False(t, write1h)
	})
	t.Run("rebuilt provider bytes drop original root marker", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(tierAdmissionProviderBodyKey, []byte(`{"messages":[{"role":"user","content":"hello"}]}`))
		write5m, write1h, err := tierAdmissionCacheWrites(tierAdmissionPreparedPayload(c, fallback))
		require.NoError(t, err)
		require.False(t, write5m)
		require.False(t, write1h)
	})
	t.Run("empty provider bytes use converted payload", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(tierAdmissionProviderBodyKey, []byte{})
		c.Set(ctxkey.ConvertedRequest, converted)
		require.Equal(t, converted, tierAdmissionPreparedPayload(c, fallback))
	})
}
