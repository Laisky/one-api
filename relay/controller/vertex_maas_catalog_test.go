package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestVertexMaaSAliasControllerWire verifies the real Chat controller flattens native controls and preserves MaaS tool schemas and history.
func TestVertexMaaSAliasControllerWire(t *testing.T) {
	const body = `{"model":"alias","messages":[{"role":"user","content":"Save a note"},{"role":"assistant","tool_calls":[{"id":"call_note","type":"function","function":{"name":"save","arguments":"{\"kind\":\"note\"}"}}]},{"role":"tool","tool_call_id":"call_note","content":"saved"}],"max_completion_tokens":128,"tools":[{"type":"function","function":{"name":"save","parameters":{"type":"object","additionalProperties":false,"properties":{"kind":{"type":"string","const":"note"},"nested":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["kind"]}}}],"extra_body":{"chat_template_kwargs":{"enable_thinking":false},"model":"wrong-route","quota":0,"unreviewed":true}}`
	for _, tc := range []struct {
		name, actual, path string
		channel            int
	}{
		{"vertex_glm52", "zai-org/glm-5.2-maas", "/v1/projects/" + providerMatrixVertexProject + "/locations/global/endpoints/openapi/chat/completions", channeltype.VertextAI},
		{"existing_openai_compatible", "controller-maas-control", "/v1/chat/completions", channeltype.OpenAICompatible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, providerMatrixBalance, false)
			type observation struct {
				path, method, authorization string
				body                        []byte
				err                         error
			}
			observed := make(chan observation, 1)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				raw, err := io.ReadAll(r.Body)
				observed <- observation{r.URL.Path, r.Method, r.Header.Get("Authorization"), raw, err}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"offline-maas","object":"chat.completion","model":"offline","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()
			c, w, _ := protocolContext(t, tc.channel, tc.actual, "/v1/chat/completions", body, server.URL, providerMatrixBalance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			c.Set(ctxkey.Config, model.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject})
			if tc.channel == channeltype.VertextAI {
				key := fmt.Sprintf("vertexai-token-%d", fallbackChannelID)
				vertexai.Cache.Set(key, providerMatrixVertexToken, time.Minute)
				defer vertexai.Cache.Delete(key)
			}
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.EqualValues(t, 1, calls.Load())
			var got observation
			select {
			case got = <-observed:
			default:
				t.Fatal("provider dispatch was not observed")
			}
			require.NoError(t, got.err)
			require.Equal(t, http.MethodPost, got.method)
			require.Equal(t, tc.path, got.path)
			auth := "Bearer upstream-fixture-key"
			if tc.channel == channeltype.VertextAI {
				auth = "Bearer " + providerMatrixVertexToken
			}
			require.Equal(t, auth, got.authorization)
			var sent, source map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(got.body, &sent))
			require.NoError(t, json.Unmarshal([]byte(body), &source))
			require.Equal(t, `"`+tc.actual+`"`, string(sent["model"]))
			for _, key := range []string{"tools", "messages"} {
				require.JSONEq(t, string(source[key]), string(sent[key]), key)
			}
			if tc.channel == channeltype.VertextAI {
				require.Equal(t, "128", string(sent["max_tokens"]))
				require.NotContains(t, sent, "max_completion_tokens")
				require.NotContains(t, sent, "quota")
				require.NotContains(t, sent, "unreviewed")
			} else {
				require.Equal(t, "128", string(sent["max_completion_tokens"]))
			}
			_, nested := sent["extra_body"]
			require.False(t, nested, "controller must flatten native extra_body into the MaaS root payload")
			require.JSONEq(t, `{"enable_thinking":false}`, string(sent["chat_template_kwargs"]))
		})
	}
}

// TestVertexNativeGeminiKeepsNativeEnvelope verifies MaaS passthrough normalization does not flatten foreign controls into native Gemini payloads.
func TestVertexNativeGeminiKeepsNativeEnvelope(t *testing.T) {
	const raw = `{"model":"alias","messages":[{"role":"user","content":"note"}],"max_completion_tokens":128,"extra_body":{"chat_template_kwargs":{"enable_thinking":false}}}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(raw))
	m := &metalib.Meta{APIType: apitype.VertexAI, ChannelType: channeltype.VertextAI, ActualModelName: "gemini-2.5-flash", OriginModelName: "alias", Mode: relaymode.ChatCompletions}
	metalib.Set2Context(c, m)
	var request relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	request.Model = m.ActualModelName
	reader, err := getRequestBody(c, m, &request, &vertexai.Adaptor{}, false)
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &wire))
	require.Contains(t, wire, "contents")
	require.Contains(t, string(wire["contents"]), "note")
	require.Contains(t, wire, "generation_config")
	require.NotContains(t, wire, "chat_template_kwargs")
	require.NotContains(t, wire, "extra_body")
	require.NotContains(t, wire, "max_tokens")
}
