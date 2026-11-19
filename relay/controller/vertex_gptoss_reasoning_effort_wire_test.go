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
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestVertexGPTOSSReasoningEffortWire drives the real Chat controller to a
// fake Vertex MaaS endpoint and asserts the reasoning_effort GPT-OSS
// receives. Vertex publishes low/medium/high for both GPT-OSS variants, so a
// listed value is forwarded in canonical lowercase and anything else is
// omitted so Vertex applies its default. It accepts t and returns no values.
func TestVertexGPTOSSReasoningEffortWire(t *testing.T) {
	const actual = "openai/gpt-oss-20b-maas"
	for _, tc := range []struct {
		name   string
		effort string
		want   string
	}{
		{name: "listed", effort: "high", want: "high"},
		{name: "case variant", effort: " HIGH ", want: "high"},
		{name: "globally valid but unlisted", effort: "max", want: ""},
		{name: "invalid", effort: "extreme", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, providerMatrixBalance, false)
			bodies := make(chan []byte, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "body read failed", http.StatusBadRequest)
					return
				}
				bodies <- raw
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"offline-gptoss","object":"chat.completion","model":"offline","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()

			effortJSON, err := json.Marshal(tc.effort)
			require.NoError(t, err)
			body := `{"model":"alias","messages":[{"role":"user","content":"hello"}],"max_tokens":64,"reasoning_effort":` + string(effortJSON) + `}`
			c, w, _ := protocolContext(t, channeltype.VertextAI, actual, "/v1/chat/completions", body, server.URL, providerMatrixBalance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			c.Set(ctxkey.Config, model.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject})
			key := fmt.Sprintf("vertexai-token-%d", fallbackChannelID)
			vertexai.Cache.Set(key, providerMatrixVertexToken, time.Minute)
			defer vertexai.Cache.Delete(key)

			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var sent map[string]json.RawMessage
			select {
			case raw := <-bodies:
				require.NoError(t, json.Unmarshal(raw, &sent))
			default:
				t.Fatal("provider dispatch was not observed")
			}
			require.Equal(t, `"`+actual+`"`, string(sent["model"]))
			if tc.want == "" {
				require.NotContains(t, sent, "reasoning_effort", "catalog-unlisted effort must be dropped so Vertex applies its default")
				return
			}
			require.JSONEq(t, `"`+tc.want+`"`, string(sent["reasoning_effort"]))
		})
	}
}
