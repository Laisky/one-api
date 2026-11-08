package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// providerMatrixByName returns the matrix provider called name. Parameters: t
// owns assertions and name is the provider name. Returns the provider.
func providerMatrixByName(t *testing.T, name string) providerMatrixProvider {
	t.Helper()
	for _, p := range providerMatrixProviders() {
		if p.name == name {
			return p
		}
	}
	require.Failf(t, "unknown matrix provider", "provider %q", name)
	return providerMatrixProvider{}
}

// providerMatrixWebSearchBody returns the synthetic Codex request with the
// hosted web_search declaration Codex always sends appended to its tools.
// Parameters: t owns assertions and stream selects SSE. Returns the JSON body.
func providerMatrixWebSearchBody(t *testing.T, stream bool) string {
	t.Helper()
	var tools []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(codexFallbackTools), &tools))
	tools = append(tools, json.RawMessage(`{"type":"web_search","external_web_access":true}`))
	encoded, err := json.Marshal(tools)
	require.NoError(t, err)
	return fmt.Sprintf(`{"model":"alias","stream":%t,"max_output_tokens":64,"tools":%s,"input":%s}`, stream, encoded, codexFallbackInput)
}

// providerMatrixAllowWebSearch returns a run option that whitelists and prices
// web_search on the fixture channel, the operator configuration admission
// requires before any fallback can consider forwarding a hosted search.
// Parameters: t owns assertions. Returns the run options.
func providerMatrixAllowWebSearch(t *testing.T) providerMatrixOptions {
	t.Helper()
	return providerMatrixOptions{prepare: func(c *gin.Context) {
		value, ok := c.Get(ctxkey.ChannelModel)
		require.True(t, ok)
		require.NoError(t, value.(*model.Channel).SetToolingConfig(claudeToolPolicyAllowSearch()))
	}}
}

// TestResponseFallbackProviderMatrixBedrockWebSearchDropped proves the
// documented Bedrock degradation (docs/manual/responses_claude_web_search.md):
// once an operator admits and prices web_search on an AwsClaude channel, the
// Codex web_search declaration is dropped rather than forwarded, because
// Bedrock runs no Claude web search and its receipts carry no search
// counters. The request still succeeds with the unchanged flattened catalog
// and lowered history, reserves no search allowance and settles tokens only.
func TestResponseFallbackProviderMatrixBedrockWebSearchDropped(t *testing.T) {
	bedrock := providerMatrixByName(t, "aws_bedrock_claude")
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			result := runProviderMatrix(t, bedrock, stream, providerMatrixWebSearchBody(t, stream), providerMatrixAllowWebSearch(t))
			requireProviderMatrixDispatched(t, result)
			requireProviderMatrixRoute(t, bedrock, stream, result.got)
			require.NotContains(t, string(result.got.Body), "web_search", "neither a server tool nor a function named web_search is forwarded")
			requireProviderMatrixUpstream(t, bedrock, result.got)
			require.Zero(t, result.c.GetInt64(ctxkey.ResponseFallbackToolAllowanceQuota), "no search allowance is reserved")
			requireResponsesRecorderConformance(t, result.recorder)
			requireProviderMatrixItems(t, bedrock, requireProviderMatrixReplyEnvelope(t, result.recorder, stream))
			requireProviderMatrixSettlement(t, result)
		})
	}
}

// TestResponseFallbackProviderMatrixWebSearchUnadmitted proves the admission
// half of the documented contract: Azure Foundry (which requires an explicit
// channel price) and Bedrock (which has no web search price at all) reject the
// Codex web_search declaration up front with 400 when the operator has not
// whitelisted and priced it, instead of silently degrading, before any
// reservation or provider dispatch.
func TestResponseFallbackProviderMatrixWebSearchUnadmitted(t *testing.T) {
	for _, name := range []string{"azure_foundry_claude", "aws_bedrock_claude"} {
		t.Run(name, func(t *testing.T) {
			p := providerMatrixByName(t, name)
			result := runProviderMatrix(t, p, false, providerMatrixWebSearchBody(t, false), providerMatrixOptions{})
			require.NotNil(t, result.apiErr, "an unpriced hosted tool must be rejected")
			require.Equal(t, 400, result.apiErr.StatusCode, fmt.Sprint(result.apiErr))
			require.Zero(t, result.calls, "a rejected request never reaches the provider")
			require.Equal(t, providerMatrixBalance, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, providerMatrixBalance, token.RemainQuota)
			require.Zero(t, token.UsedQuota)
		})
	}
}

// TestResponseFallbackProviderMatrixWebSearchForwarded is the contrast to the
// Bedrock drop: the Claude platforms that run and meter Claude web search
// forward the same Codex declaration as exactly one server tool of their
// documented version (dynamic web_search_20260209 for first-party Claude
// 4.6+, basic web_search_20250305 on Foundry and Vertex) with max_uses 10,
// keep the whole flattened catalog, reserve the search allowance before
// dispatch and refund it on a receipt reporting zero searches. Anthropic and
// Vertex price web search by default; Foundry needs the operator price.
func TestResponseFallbackProviderMatrixWebSearchForwarded(t *testing.T) {
	for _, tc := range []struct {
		name, toolType string
		priced         bool
	}{
		{name: "anthropic", toolType: "web_search_20260209"},
		{name: "azure_foundry_claude", toolType: "web_search_20250305", priced: true},
		{name: "vertex_claude", toolType: "web_search_20250305"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := providerMatrixByName(t, tc.name)
			opts := providerMatrixOptions{}
			if tc.priced {
				opts = providerMatrixAllowWebSearch(t)
			}
			opts.reply = &providerMatrixReply{contentType: "application/json", body: []byte(`{"id":"msg_matrix","type":"message","role":"assistant",` +
				`"model":"claude-opus-4-6","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn",` +
				`"usage":{"input_tokens":11,"output_tokens":7,"server_tool_use":{"web_search_requests":0}}}`)}
			result := runProviderMatrix(t, p, false, providerMatrixWebSearchBody(t, false), opts)
			requireProviderMatrixDispatched(t, result)
			requireProviderMatrixRoute(t, p, false, result.got)

			var request struct {
				Tools []map[string]json.RawMessage `json:"tools"`
			}
			require.NoError(t, json.Unmarshal(result.got.Body, &request))
			var searches []map[string]json.RawMessage
			for _, tool := range request.Tools {
				if string(tool["name"]) == `"web_search"` {
					searches = append(searches, tool)
				}
			}
			require.Len(t, searches, 1, "exactly one forwarded server search tool")
			require.JSONEq(t, `"`+tc.toolType+`"`, string(searches[0]["type"]))
			require.JSONEq(t, `10`, string(searches[0]["max_uses"]))
			wire := decodeFallbackWire(t, channeltype.Anthropic, result.got.Body)
			require.Equal(t, append(append([]string(nil), codexFallbackFlatTools...), "web_search"), fallbackWireToolNames(wire),
				"the flattened catalog is forwarded unchanged ahead of the server tool")
			requireFallbackWireTurns(t, codexFallbackExpectedTurns(), wire.Turns)

			require.Positive(t, result.c.GetInt64(ctxkey.ResponseFallbackToolAllowanceQuota), "the search allowance is reserved")
			requireResponsesRecorderConformance(t, result.recorder)
			output := requireProviderMatrixReplyEnvelope(t, result.recorder, false)
			require.Len(t, output, 1)
			requireProviderMatrixSettlement(t, result)
		})
	}
}

// providerMatrixImageBody returns a Responses request whose single user turn
// carries text and an inline PNG. Parameters: data is the base64 PNG. Returns
// the JSON body.
func providerMatrixImageBody(data string) string {
	return fmt.Sprintf(`{"model":"alias","stream":false,"max_output_tokens":64,"input":[{"type":"message","role":"user","content":[`+
		`{"type":"input_text","text":"Describe the image."},{"type":"input_image","image_url":"data:image/png;base64,%s","detail":"high"}]}]}`, data)
}

// providerMatrixTextReply returns provider p's plain "done" reply with usage
// of 11 prompt and 7 completion tokens. Parameters: p is the provider.
// Returns the encoded reply.
func providerMatrixTextReply(p providerMatrixProvider) providerMatrixReply {
	if p.family == providerMatrixGemini {
		return providerMatrixReply{contentType: "application/json", body: []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},` +
			`"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18},"modelVersion":"gemini-matrix-fixture"}`)}
	}
	recorder := httptest.NewRecorder()
	writeFallbackTextReply(recorder, providerMatrixWireChannel(p))
	return providerMatrixReply{contentType: recorder.Header().Get("Content-Type"), body: recorder.Body.Bytes()}
}

// requireProviderMatrixNativeImage asserts the first upstream user turn
// carries the client text and the PNG in the provider's native image form:
// Gemini inlineData, a Claude base64 image block, or a Chat image_url data
// URL. Parameters: t owns assertions, p is the provider, body the upstream
// request and data the base64 PNG.
func requireProviderMatrixNativeImage(t *testing.T, p providerMatrixProvider, body []byte, data string) {
	t.Helper()
	switch p.family {
	case providerMatrixGemini:
		_, request := decodeProviderMatrixGeminiWire(t, body)
		require.NotEmpty(t, request.Contents)
		first := request.Contents[0]
		require.Equal(t, "user", first.Role)
		var text string
		var images int
		for _, part := range first.Parts {
			text += part.Text
			if part.InlineData != nil {
				images++
				require.Equal(t, "image/png", part.InlineData.MimeType)
				require.Equal(t, data, part.InlineData.Data, "Gemini receives the image bytes inline")
			}
		}
		require.Equal(t, "Describe the image.", text)
		require.Equal(t, 1, images, "exactly one inlineData part")
	case providerMatrixClaude:
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type   string `json:"type"`
					Text   string `json:"text"`
					Source *struct {
						Type      string `json:"type"`
						MediaType string `json:"media_type"`
						Data      string `json:"data"`
					} `json:"source"`
				} `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(body, &request))
		require.Len(t, request.Messages, 1)
		require.Equal(t, "user", request.Messages[0].Role)
		require.Len(t, request.Messages[0].Content, 2)
		require.Equal(t, "Describe the image.", request.Messages[0].Content[0].Text)
		image := request.Messages[0].Content[1]
		require.Equal(t, "image", image.Type)
		require.NotNil(t, image.Source)
		require.Equal(t, "base64", image.Source.Type)
		require.Equal(t, "image/png", image.Source.MediaType)
		require.Equal(t, data, image.Source.Data)
	default:
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL *struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, json.Unmarshal(body, &request))
		require.Len(t, request.Messages, 1)
		require.Equal(t, "user", request.Messages[0].Role)
		require.Len(t, request.Messages[0].Content, 2)
		require.Equal(t, "Describe the image.", request.Messages[0].Content[0].Text)
		image := request.Messages[0].Content[1]
		require.Equal(t, "image_url", image.Type)
		require.NotNil(t, image.ImageURL)
		require.Equal(t, "data:image/png;base64,"+data, image.ImageURL.URL)
	}
}

// TestResponseFallbackProviderMatrixImageInput relays one Responses input_image
// through every fallback provider and proves each upstream receives the PNG in
// its native form (Gemini inlineData, Claude base64 image block, Chat data
// URL), the reply passes wire conformance and tokens settle once.
func TestResponseFallbackProviderMatrixImageInput(t *testing.T) {
	data := providerImageFixture(t)
	for _, p := range providerMatrixProviders() {
		t.Run(p.name, func(t *testing.T) {
			reply := providerMatrixTextReply(p)
			result := runProviderMatrix(t, p, false, providerMatrixImageBody(data), providerMatrixOptions{reply: &reply})
			requireProviderMatrixDispatched(t, result)
			requireProviderMatrixRoute(t, p, false, result.got)
			requireProviderMatrixNativeImage(t, p, result.got.Body, data)
			requireResponsesRecorderConformance(t, result.recorder)
			output := requireProviderMatrixReplyEnvelope(t, result.recorder, false)
			require.Len(t, output, 1)
			var message struct {
				Type    string `json:"type"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			}
			require.NoError(t, json.Unmarshal(output[0], &message))
			require.Equal(t, "message", message.Type)
			require.Len(t, message.Content, 1)
			require.Equal(t, "done", message.Content[0].Text)
			requireProviderMatrixSettlement(t, result)
		})
	}
}
