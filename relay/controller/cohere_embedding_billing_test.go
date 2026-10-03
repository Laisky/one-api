package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestProtocolAuditCohereLedger exercises the full embeddings relay, real SQLite
// billing, aliases, all five catalog models, operator overrides and refunds.
func TestProtocolAuditCohereLedger(t *testing.T) {
	for _, tc := range []struct {
		name, actual string
		status       int
		group        float64
		quota        int64
		override     *model.ModelConfigLocal
		disconnect   bool
	}{
		{"v4", "embed-v4.0", 200, 1, 60, nil, false},
		{"english", "embed-english-v3.0", 200, 1, 50, nil, false},
		{"english_light", "embed-english-light-v3.0", 200, 1, 50, nil, false},
		{"multilingual", "embed-multilingual-v3.0", 200, 1, 50, nil, false},
		{"multilingual_light", "embed-multilingual-light-v3.0", 200, 1, 50, nil, false},
		{"group", "embed-v4.0", 200, 2, 120, nil, false},
		{"free_group", "embed-v4.0", 200, 0, 0, nil, false},
		{"override", "embed-v4.0", 200, 1, 200, &model.ModelConfigLocal{Ratio: .2}, false},
		{"refused", "embed-v4.0", 403, 1, 0, nil, false},
		{"limited", "embed-v4.0", 429, 1, 0, nil, false},
		{"overloaded", "embed-v4.0", 503, 1, 0, nil, false},
		{"client_write_failure", "embed-v4.0", 200, 1, 60, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100000)
			xaiVideoSetup(t, balance, false)
			observations := make(chan map[string]any, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var wire map[string]any
				err := json.NewDecoder(r.Body).Decode(&wire)
				observations <- map[string]any{"body": wire, "error": err, "path": r.URL.Path, "auth": r.Header.Get("Authorization")}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.status != 200 {
					_, _ = io.WriteString(w, `{"message":"upstream rejected request"}`)
					return
				}
				_, _ = io.WriteString(w, `{"embeddings":{"float":[[0.1,0.2],[0.3,0.4]]},"meta":{"billed_units":{"input_tokens":1000},"tokens":{"input_tokens":9900}}}`)
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			c, w, id := protocolContext(t, channeltype.Cohere, tc.actual, "/v1/embeddings", `{"model":"alias","input":["hello","world"],"input_type":"search_query"}`, server.URL, balance, tc.group, false, tc.override)
			if tc.disconnect {
				c.Writer = xaiDisconnectedWriter{ResponseWriter: c.Writer}
			}
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			require.Equal(t, balance-tc.quota, reloadUserQuota(t))
			require.Equal(t, tc.quota, requestCostQuota(t, id))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, tc.quota, token.UsedQuota)
			observed := <-observations
			require.Nil(t, observed["error"])
			require.Equal(t, "/v2/embed", observed["path"])
			require.Equal(t, "Bearer upstream-fixture-key", observed["auth"])
			wire := observed["body"].(map[string]any)
			require.Equal(t, tc.actual, wire["model"])
			require.Equal(t, []any{"hello", "world"}, wire["texts"])
			require.Equal(t, "search_query", wire["input_type"])
			if tc.status != 200 || tc.disconnect {
				require.NotNil(t, apiErr)
				return
			}
			require.Nil(t, apiErr)
			var result map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
			require.Equal(t, "alias", result["model"])
			require.Len(t, result["data"], 2)
			require.Equal(t, float64(1000), result["usage"].(map[string]any)["prompt_tokens"])
		})
	}
}

// TestCohereInvalidInputDoesNotSpend verifies validation precedes provider access
// and physical quota is restored when conversion fails after admission.
func TestCohereInvalidInputDoesNotSpend(t *testing.T) {
	const balance = int64(10000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	c, _, _ := protocolContext(t, channeltype.Cohere, "embed-v4.0", "/v1/embeddings", `{"model":"alias","input":[1,2,3]}`, server.URL, balance, 1, false, nil)
	apiErr := RelayTextHelper(c)
	require.NotNil(t, apiErr)
	drainCriticalTasks(t)
	require.Zero(t, calls.Load())
	require.Equal(t, balance, reloadUserQuota(t))
	require.True(t, strings.Contains(apiErr.Message, "embedding") || strings.Contains(apiErr.Message, "input"), apiErr.Message)
}
