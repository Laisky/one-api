package openai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	relaymeta "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestAzureGPT6ResponseRouting verifies that published Azure GPT-6 deployments
// use the Responses endpoint for Chat, Claude, and native Responses requests.
func TestAzureGPT6ResponseRouting(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-6-astra", "gpt-6-sol", "gpt-6-luna", "gpt-6.1-sol"} {
		for _, surface := range []struct {
			name string
			mode int
			path string
		}{
			{"chat", relaymode.ChatCompletions, "/v1/chat/completions"},
			{"claude", relaymode.ClaudeMessages, "/v1/messages"},
			{"responses", relaymode.ResponseAPI, "/v1/responses"},
		} {
			t.Run(name+"/"+surface.name, func(t *testing.T) {
				info := &relaymeta.Meta{
					ChannelType: channeltype.Azure, ActualModelName: name,
					Mode: surface.mode, RequestURLPath: surface.path,
					BaseURL: "https://example.openai.azure.com",
				}
				require.True(t, AzureRequiresResponseAPI(name))
				require.True(t, shouldForceResponseAPI(info))
				a := &Adaptor{ChannelType: channeltype.Azure}
				url, err := a.GetRequestURL(info)
				require.NoError(t, err)
				require.Equal(t, "https://example.openai.azure.com/openai/v1/responses?api-version=v1", url)
			})
		}
	}
}

// TestAzureGPT6RoutingPreservesOtherDeployments verifies that adding known GPT-6
// IDs does not classify arbitrary deployment names or GPT-OSS as Responses-only.
func TestAzureGPT6RoutingPreservesOtherDeployments(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-4o", "gpt-oss-120b", "gpt-60", "gpt-6.1-sol-custom", "tenant-deployment"} {
		require.False(t, AzureRequiresResponseAPI(name), name)
	}
	require.True(t, AzureRequiresResponseAPI(" GPT-6.1-SOL "))
	require.True(t, AzureRequiresResponseAPI("gpt-5.4"))
}

// TestAzureGPT6ChatConversion verifies the actual Azure conversion serializes
// reasoning and function tools into Responses rather than forwarding Chat JSON.
func TestAzureGPT6ChatConversion(t *testing.T) {
	for _, name := range []string{"gpt-6-astra", "gpt-6-sol", "gpt-6-luna", "gpt-6.1-sol"} {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaymeta.Meta{
				ChannelType: channeltype.Azure, ActualModelName: name,
				Mode: relaymode.ChatCompletions, RequestURLPath: "/v1/chat/completions",
			}
			relaymeta.Set2Context(c, info)
			var request model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(`{
				"messages":[{"role":"user","content":"Find the release notes."}],
				"reasoning_effort":"xhigh", "temperature":0, "top_p":0.9,
				"tools":[{"type":"function","function":{"name":"lookup","parameters":{
					"type":"object","properties":{},"additionalProperties":false
				}}}]
			}`), &request))
			request.Model = name
			a := &Adaptor{ChannelType: channeltype.Azure}
			converted, err := a.ConvertRequest(c, relaymode.ChatCompletions, &request)
			require.NoError(t, err)
			require.IsType(t, &ResponseAPIRequest{}, converted)
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			var root map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &root))
			require.Contains(t, root, "input")
			require.NotContains(t, root, "messages")
			require.NotContains(t, root, "temperature")
			require.NotContains(t, root, "top_p")
			var reasoning map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(root["reasoning"], &reasoning))
			require.JSONEq(t, `"xhigh"`, string(reasoning["effort"]))
			var tools []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(root["tools"], &tools))
			require.Len(t, tools, 1)
			require.JSONEq(t, `"lookup"`, string(tools[0]["name"]))
		})
	}
}
