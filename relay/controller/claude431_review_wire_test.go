package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestClaude431ReviewWireIsolation drives the four serialized request paths against a strict fixture.
// Shared DTOs cannot forward newly preserved native Claude fields to an OpenAI wire provider.
func TestClaude431ReviewWireIsolation(t *testing.T) {
	ensureResponseFallbackFixtures(t)
	oldRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(oldRedis) })
	oldLogging := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(false)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLogging) })
	for _, path := range []string{"/v1/messages", "/v1/chat/completions", "/v1/responses", "mcp"} {
		t.Run(path, func(t *testing.T) {
			seedRetryDoubleChargeUser(t)
			var mu sync.Mutex
			var bodies [][]byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "body read failed", 400)
					return
				}
				mu.Lock()
				bodies = append(bodies, raw)
				mu.Unlock()
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(raw, &fields); err != nil {
					http.Error(w, "invalid JSON", 400)
					return
				}
				if _, found := fields["output_config"]; found || strings.Contains(string(fields["thinking"]), "display") || strings.Contains(string(fields["thinking"]), "native_extension") || strings.Contains(string(fields["thinking"]), "block_binding") {
					http.Error(w, "unsupported Claude-only field", 400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"chatcmpl_fixture","object":"chat.completion","model":"anthropic/claude-sonnet-5.5","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`)
			}))
			defer upstream.Close()
			oldClient := client.HTTPClient
			client.HTTPClient = upstream.Client()
			defer func() { client.HTTPClient = oldClient }()
			const id = "anthropic/claude-sonnet-5.5"
			control := `"thinking":{"type":"adaptive","display":"omitted","native_extension":{"id":9007199254740993},"block_binding":{"prefix_mismatch_behavior":"error"}},"output_config":{"effort":"high"}`
			payload := `{"model":"` + id + `","max_tokens":1024,"messages":[{"role":"user","content":"hello"}],` + control + `}`
			if path == "/v1/responses" {
				payload = `{"model":"` + id + `","max_output_tokens":1024,"input":"hello",` + control + `}`
			}
			w := httptest.NewRecorder()
			c := setupClaudeRetryContext(t, w, upstream.URL)
			c.Set(ctxkey.Channel, channeltype.OpenRouter)
			c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackAnthropicChannelID, Type: channeltype.OpenRouter})
			c.Set(ctxkey.RequestModel, id)
			target := path
			if target == "mcp" {
				target = "/v1/chat/completions"
			}
			c.Request = httptest.NewRequest(http.MethodPost, target, strings.NewReader(payload))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("Authorization", "Bearer fixture-key")
			var failure *relaymodel.ErrorWithStatusCode
			switch path {
			case "/v1/messages":
				failure = RelayClaudeMessagesHelper(c)
			case "/v1/chat/completions":
				failure = RelayTextHelper(c)
			case "/v1/responses":
				failure = RelayResponseAPIHelper(c)
			case "mcp":
				var request relaymodel.GeneralOpenAIRequest
				require.NoError(t, json.Unmarshal([]byte(payload), &request))
				m := meta.GetByContext(c)
				m.Mode = relaymode.ChatCompletions
				_, _, failure = doChatRequestOnce(c, m, &openrouter.Adaptor{}, &request)
				require.NotEmpty(t, request.OutputConfig)
				require.NotEmpty(t, request.Thinking.ExtraFields)
			}
			require.Nil(t, failure, "strict provider must receive only its wire fields: %v", failure)
			drainCriticalTasks(t)
			mu.Lock()
			captured := append([][]byte(nil), bodies...)
			mu.Unlock()
			require.Len(t, captured, 1)
			var outgoing map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(captured[0], &outgoing))
			require.JSONEq(t, fmt.Sprintf("%q", id), string(outgoing["model"]))
			require.NotContains(t, outgoing, "output_config")
		})
	}
}

// TestClaude431ReviewExplicitRawPassthrough preserves a custom upstream's existing opt-in raw contract.
// Cleanup at typed serialization boundaries must not invent restrictions for opaque custom providers.
func TestClaude431ReviewExplicitRawPassthrough(t *testing.T) {
	old := config.EnforceIncludeUsage
	config.EnforceIncludeUsage = false
	t.Cleanup(func() { config.EnforceIncludeUsage = old })
	raw := `{"model":"custom-model","messages":[{"role":"user","content":"hello"}],"thinking":{"type":"adaptive","native_extension":true},"output_config":{"effort":"high"}}`
	var request relaymodel.GeneralOpenAIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &request))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw))
	m := &meta.Meta{ChannelType: channeltype.OpenAICompatible, APIType: apitype.OpenAI, OriginModelName: "custom-model", ActualModelName: "custom-model"}
	body, err := getRequestBody(c, m, &request, &openrouter.Adaptor{}, false)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, raw, string(encoded))
}
