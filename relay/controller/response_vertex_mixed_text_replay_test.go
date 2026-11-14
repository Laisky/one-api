package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// vertexMixedTextProvider selects the existing offline provider fixture for native Vertex Gemini.
func vertexMixedTextProvider(t *testing.T) providerMatrixProvider {
	t.Helper()
	for _, p := range providerMatrixProviders() {
		if p.name == "gemini" {
			p.name = "vertex_gemini"
			p.channel = channeltype.VertextAI
			p.config = dbmodel.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject, Region: providerMatrixVertexRegion}
			p.vertexToken = providerMatrixVertexToken
			return p
		}
	}
	t.Fatal("native Gemini fixture missing")
	return providerMatrixProvider{}
}

// TestResponsesVertexMixedTextToolReplay verifies actual buffered/SSE relay round trips restore source part order and independent signatures.
func TestResponsesVertexMixedTextToolReplay(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "buffered"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			p := vertexMixedTextProvider(t)
			const candidate = `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"thinking","thought":true,"thoughtSignature":"text-sig"},{"functionCall":{"name":"one","args":{}},"thoughtSignature":"call-sig"},{"text":"","thoughtSignature":"empty-sig"},{"functionCall":{"name":"two","args":{}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"totalTokenCount":18}}`
			reply := providerMatrixReply{contentType: "application/json", body: []byte(candidate)}
			if stream {
				reply.contentType = "text/event-stream"
				reply.body = []byte("data: " + candidate + "\n\ndata: [DONE]\n\n")
			}
			body := `{"model":"` + p.actual + `","input":"begin","tools":[{"type":"function","name":"one","parameters":{"type":"object"}},{"type":"function","name":"two","parameters":{"type":"object"}}]}`
			if stream {
				body = `{"stream":true,` + body[1:]
			}
			result := runProviderMatrix(t, p, stream, body, providerMatrixOptions{reply: &reply})
			requireProviderMatrixDispatched(t, result)
			var response openai.ResponseAPIResponse
			if stream {
				events := bridgeFindEvents(parseBridgeSSE(result.recorder.Body.String()), "response.completed")
				require.Len(t, events, 1)
				var event openai.ResponseAPIStreamEvent
				bridgeUnmarshal(t, events[0], &event)
				response = *event.Response
			} else {
				require.NoError(t, json.Unmarshal(result.recorder.Body.Bytes(), &response))
			}
			var input []any
			encoded, err := json.Marshal(response.Output)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(encoded, &input))
			calls := 0
			for _, item := range response.Output {
				if item.Type == "function_call" {
					calls++
					input = append(input, map[string]any{"type": "function_call_output", "call_id": item.CallId, "output": "done"})
				}
			}
			require.Equal(t, 2, calls)
			replayBody, err := json.Marshal(map[string]any{"model": p.actual, "input": input})
			require.NoError(t, err)
			replay := runProviderMatrix(t, p, false, string(replayBody), providerMatrixOptions{})
			requireProviderMatrixDispatched(t, replay)
			var native struct {
				Contents []struct {
					Role  string           `json:"role"`
					Parts []map[string]any `json:"parts"`
				} `json:"contents"`
			}
			require.NoError(t, json.Unmarshal(replay.got.Body, &native))
			var parts []map[string]any
			for _, content := range native.Contents {
				if content.Role == "model" {
					parts = append(parts, content.Parts...)
				}
			}
			require.Len(t, parts, 4)
			require.Equal(t, "thinking", parts[0]["text"])
			require.Equal(t, true, parts[0]["thought"])
			require.Equal(t, "text-sig", parts[0]["thoughtSignature"])
			require.Equal(t, "one", parts[1]["functionCall"].(map[string]any)["name"])
			require.Equal(t, "call-sig", parts[1]["thoughtSignature"])
			require.Equal(t, "empty-sig", parts[2]["thoughtSignature"])
			require.Equal(t, "", parts[2]["text"], "signed empty text must retain its protobuf oneof field")
			require.Equal(t, "two", parts[3]["functionCall"].(map[string]any)["name"])
			require.NotContains(t, parts[3], "thoughtSignature", "unsigned calls must remain unsigned")
			for _, part := range parts {
				require.NotContains(t, part, "extra_content")
			}
		})
	}
}
