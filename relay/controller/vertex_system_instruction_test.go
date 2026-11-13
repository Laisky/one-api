package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestVertexMappedSystemInstructionWire exercises each public ingress with native tool continuation.
// The loopback fixture records the actual mapped HTTP body without credentials or live Google calls.
func TestVertexMappedSystemInstructionWire(t *testing.T) {
	for _, actual := range []string{"gemini-4-hydrogen", "gemini-4-carbon", "gemini-4-carbon-cyber-permissive", "gemini-4-argon-cyber-permissive", "gemini-3.8-flash-cyber", "gemini-2.5-flash"} {
		for _, ingress := range []string{"chat", "claude", "responses"} {
			t.Run(actual+"/"+ingress, func(t *testing.T) {
				xaiVideoSetup(t, providerMatrixBalance, false)
				const history = `[{"role":"user","content":"Begin"},{"role":"assistant","tool_calls":[{"id":"call_lookup","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_lookup","content":"done"},{"role":"user","content":"Continue"}]`
				path, body := "/v1/chat/completions", `{"model":"alias","max_tokens":128,"messages":[{"role":"system","content":"Answer in French."},`+history[1:]
				body += "}"
				if ingress == "claude" {
					path = "/v1/messages"
					body = `{"model":"alias","max_tokens":128,"system":"Answer in French.","messages":[{"role":"user","content":"Begin"},{"role":"assistant","content":[{"type":"tool_use","id":"call_lookup","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_lookup","content":"done"}]},{"role":"user","content":"Continue"}]}`
				}
				if ingress == "responses" {
					path = "/v1/responses"
					body = `{"model":"alias","max_output_tokens":128,"instructions":"Answer in French.","input":[{"type":"message","role":"user","content":"Begin"},{"type":"function_call","call_id":"call_lookup","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_lookup","output":"done"},{"type":"message","role":"user","content":"Continue"}]}`
				}
				observed := make(chan []byte, 1)
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, "/v1/projects/"+providerMatrixVertexProject+"/locations/global/publishers/google/models/"+actual+":generateContent", r.URL.Path)
					raw, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					observed <- raw
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"Bonjour"}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`)
				}))
				defer server.Close()
				previous := client.HTTPClient
				client.HTTPClient = server.Client()
				defer func() { client.HTTPClient = previous }()
				c, w, _ := protocolContext(t, channeltype.VertextAI, actual, path, body, server.URL, providerMatrixBalance, 1, false, &dbmodel.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, CachedInputRatio: .3})
				c.Set(ctxkey.Config, dbmodel.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject})
				cacheKey := fmt.Sprintf("vertexai-token-%d", fallbackChannelID)
				vertexai.Cache.Set(cacheKey, providerMatrixVertexToken, time.Minute)
				defer vertexai.Cache.Delete(cacheKey)
				switch ingress {
				case "chat":
					require.Nil(t, RelayTextHelper(c))
				case "claude":
					require.Nil(t, RelayClaudeMessagesHelper(c))
				case "responses":
					require.Nil(t, RelayResponseAPIHelper(c))
				}
				drainCriticalTasks(t)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				var raw []byte
				select {
				case raw = <-observed:
				default:
					t.Fatal("native dispatch not observed")
				}
				var wire struct {
					System *struct {
						Role  string `json:"role"`
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"system_instruction"`
					Contents []struct {
						Role  string           `json:"role"`
						Parts []map[string]any `json:"parts"`
					} `json:"contents"`
				}
				require.NoError(t, json.Unmarshal(raw, &wire))
				require.NotNil(t, wire.System, "system instructions must retain native priority rather than becoming ordinary history")
				require.Empty(t, wire.System.Role)
				require.Len(t, wire.System.Parts, 1)
				require.Equal(t, "Answer in French.", wire.System.Parts[0].Text)
				require.Len(t, wire.Contents, 4, "system promotion and function-result alignment must agree")
				require.Equal(t, "user", wire.Contents[0].Role)
				require.Equal(t, "model", wire.Contents[1].Role)
				require.Contains(t, wire.Contents[1].Parts[0], "functionCall")
				require.Equal(t, "user", wire.Contents[2].Role)
				require.Equal(t, "lookup", wire.Contents[2].Parts[0]["functionResponse"].(map[string]any)["name"])
				encoded, err := json.Marshal(wire.Contents)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "Answer in French.")
				require.NotContains(t, string(encoded), "Okay")
				require.Contains(t, string(encoded), "Continue")
			})
		}
	}
}
