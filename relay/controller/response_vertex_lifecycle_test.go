package controller

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// vertexLifecycleProvider describes the offline native v1 route and synthetic authorization.
func vertexLifecycleProvider() providerMatrixProvider {
	const name = "gemini-3.8-flash"
	base := "/v1/projects/" + providerMatrixVertexProject + "/locations/global/publishers/google/models/" + name
	return providerMatrixProvider{name: "vertex_lifecycle", channel: channeltype.VertextAI, actual: name, family: providerMatrixGemini,
		config: dbmodel.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject, Region: providerMatrixVertexRegion}, vertexToken: providerMatrixVertexToken,
		path: providerMatrixLiteralPath(base+":generateContent", base+":streamGenerateContent"),
		auth: providerMatrixHeaderEquals("Authorization", "Bearer "+providerMatrixVertexToken),
		query: func(stream bool) string {
			if stream {
				return "alt=sse"
			}
			return ""
		},
	}
}

// vertexLifecycleResponse extracts the terminal Responses result from buffered JSON or SSE.
func vertexLifecycleResponse(t *testing.T, result providerMatrixResult, stream bool) openai.ResponseAPIResponse {
	t.Helper()
	var response openai.ResponseAPIResponse
	if !stream {
		require.NoError(t, json.Unmarshal(result.recorder.Body.Bytes(), &response))
		return response
	}
	events := bridgeFindEvents(parseBridgeSSE(result.recorder.Body.String()), "response.completed")
	require.Len(t, events, 1, result.recorder.Body.String())
	var event openai.ResponseAPIStreamEvent
	bridgeUnmarshal(t, events[0], &event)
	require.NotNil(t, event.Response)
	return *event.Response
}

// TestVertexResponsesSignedToolLifecycle exercises two real relay turns through native v1 JSON and SSE mocks.
func TestVertexResponsesSignedToolLifecycle(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			p := vertexLifecycleProvider()
			const a = `{"functionCall":{"name":"record","args":{"slot":1}},"thoughtSignature":"c2lnLWE="}`
			const b = `{"functionCall":{"name":"record"},"thoughtSignature":"c2lnLWI="}`
			const usage = `"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}`
			firstReply := providerMatrixReply{contentType: "application/json", body: []byte(`{"candidates":[{"index":0,"content":{"role":"model","parts":[` + a + `,` + b + `]},"finishReason":"STOP"}],` + usage + `}`)}
			if stream {
				firstReply.contentType = "text/event-stream"
				firstReply.body = []byte("data: " + `{"candidates":[{"index":0,"content":{"role":"model","parts":[` + a + `]}}]}` + "\n\n" + "data: " + `{"candidates":[{"index":0,"content":{"role":"model","parts":[` + b + `]},"finishReason":"STOP"}],` + usage + `}` + "\n\n")
			}
			tools := []any{map[string]any{"type": "function", "name": "record", "parameters": map[string]any{"type": "object", "properties": map[string]any{"slot": map[string]any{"type": "integer"}}}}}
			body, err := json.Marshal(map[string]any{"model": p.actual, "stream": stream, "input": "Record both slots", "tools": tools, "tool_choice": "required"})
			require.NoError(t, err)
			first := runProviderMatrix(t, p, stream, string(body), providerMatrixOptions{reply: &firstReply})
			requireProviderMatrixDispatched(t, first)
			requireProviderMatrixRoute(t, p, stream, first.got)
			var sent map[string]any
			require.NoError(t, json.Unmarshal(first.got.Body, &sent))
			require.Equal(t, "ANY", sent["tool_config"].(map[string]any)["function_calling_config"].(map[string]any)["mode"])
			response := vertexLifecycleResponse(t, first, stream)
			require.Len(t, response.Output, 2)
			assertVertexCallSignature(t, response.Output[0], response.Output[0].CallId, "c2lnLWE=")
			assertVertexCallSignature(t, response.Output[1], response.Output[1].CallId, "c2lnLWI=")
			require.NotEqual(t, response.Output[0].CallId, response.Output[1].CallId)
			input := []any{map[string]any{"role": "user", "content": "Record both slots"}, response.Output[0], response.Output[1],
				map[string]any{"type": "function_call_output", "call_id": response.Output[1].CallId, "output": `{"receipt":2}`},
				map[string]any{"type": "function_call_output", "call_id": response.Output[0].CallId, "output": `{"receipt":1}`}}
			body, err = json.Marshal(map[string]any{"model": p.actual, "stream": stream, "input": input, "tools": tools, "tool_choice": "none"})
			require.NoError(t, err)
			secondReply := providerMatrixReply{contentType: "application/json", body: []byte(`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Completed"}]},"finishReason":"STOP"}],` + usage + `}`)}
			if stream {
				secondReply.contentType = "text/event-stream"
				secondReply.body = []byte("data: " + string(secondReply.body) + "\n\n")
			}
			second := runProviderMatrix(t, p, stream, string(body), providerMatrixOptions{reply: &secondReply})
			requireProviderMatrixDispatched(t, second)
			requireProviderMatrixRoute(t, p, stream, second.got)
			require.NoError(t, json.Unmarshal(second.got.Body, &sent))
			require.Equal(t, "NONE", sent["tool_config"].(map[string]any)["function_calling_config"].(map[string]any)["mode"])
			contents := sent["contents"].([]any)
			require.Len(t, contents, 3)
			calls := contents[1].(map[string]any)["parts"].([]any)
			require.Len(t, calls, 2)
			for i, signature := range []string{"c2lnLWE=", "c2lnLWI="} {
				part := calls[i].(map[string]any)
				require.Equal(t, signature, part["thoughtSignature"])
				require.NotContains(t, part["functionCall"], "id")
			}
			require.Equal(t, map[string]any{}, calls[1].(map[string]any)["functionCall"].(map[string]any)["args"])
			require.NotContains(t, string(second.got.Body), "gemini_part_index")
			require.NotContains(t, string(second.got.Body), "extra_content")
			results := contents[2].(map[string]any)["parts"].([]any)
			require.Len(t, results, 2)
			for i, part := range results {
				fr := part.(map[string]any)["functionResponse"].(map[string]any)
				require.Equal(t, "record", fr["name"])
				require.Equal(t, map[string]any{"receipt": float64(i + 1)}, fr["response"])
				require.NotContains(t, fr, "id")
			}
			final := vertexLifecycleResponse(t, second, stream)
			require.Equal(t, "completed", final.Status)
			require.NotNil(t, final.Usage)
			require.Equal(t, 13, final.Usage.TotalTokens)
		})
	}
}
