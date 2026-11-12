package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
)

// TestVertexResponsesStoredSignedToolLifecycle verifies previous_response_id replays stored native signatures.
// Both relay turns use the real scoped state commit/hydrator and local JSON/SSE transports;
// the continuation supplies results only, so prior calls can come only from stored state.
func TestVertexResponsesStoredSignedToolLifecycle(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			store := enableStateForTest(t)
			p := vertexLifecycleProvider()
			const calls = `{"functionCall":{"name":"record","args":{"slot":1}},"thoughtSignature":"c3RvcmVkLWE="},{"functionCall":{"name":"record","args":{"slot":2}},"thoughtSignature":"c3RvcmVkLWI="}`
			const usage = `"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}`
			firstReply := providerMatrixReply{contentType: "application/json", body: []byte(`{"candidates":[{"index":0,"content":{"role":"model","parts":[` + calls + `]},"finishReason":"STOP"}],` + usage + `}`)}
			if stream {
				firstReply.contentType = "text/event-stream"
				firstReply.body = []byte("data: " + string(firstReply.body) + "\n\n")
			}
			tools := []any{map[string]any{"type": "function", "name": "record", "parameters": map[string]any{"type": "object", "properties": map[string]any{"slot": map[string]any{"type": "integer"}}}}}
			var first providerMatrixResult
			var firstResponse openai.ResponseAPIResponse
			require.True(t, t.Run("store", func(t *testing.T) {
				body, err := json.Marshal(map[string]any{"model": p.actual, "stream": stream, "store": true, "input": "Record both slots", "tools": tools, "tool_choice": "required"})
				require.NoError(t, err)
				first = runProviderMatrix(t, p, stream, string(body), providerMatrixOptions{reply: &firstReply})
				requireProviderMatrixDispatched(t, first)
				firstResponse = vertexLifecycleResponse(t, first, stream)
				require.True(t, state.LooksLikeGatewayResponseID(firstResponse.Id), firstResponse.Id)
				require.Len(t, firstResponse.Output, 2)
			}))
			owner := stateOwnerFromMeta(metalib.GetByContext(first.c))
			recorded, err := store.GetResponse(context.Background(), owner, firstResponse.Id)
			require.NoError(t, err)
			require.NotNil(t, recorded.Binding)
			require.Equal(t, p.actual, recorded.Binding.ActualModel)
			require.Len(t, recorded.OutputItems, 2)
			for i, signature := range []string{"c3RvcmVkLWE=", "c3RvcmVkLWI="} {
				var output openai.OutputItem
				require.NoError(t, json.Unmarshal(recorded.OutputItems[i].Raw, &output))
				assertVertexCallSignature(t, output, firstResponse.Output[i].CallId, signature)
			}
			// Do not resend the original user input, function calls or signature extension.
			input := []any{
				map[string]any{"type": "function_call_output", "call_id": firstResponse.Output[1].CallId, "output": `{"receipt":2}`},
				map[string]any{"type": "function_call_output", "call_id": firstResponse.Output[0].CallId, "output": `{"receipt":1}`},
			}
			secondReply := providerMatrixReply{contentType: "application/json", body: []byte(`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"Stored continuation complete"}]},"finishReason":"STOP"}],` + usage + `}`)}
			if stream {
				secondReply.contentType = "text/event-stream"
				secondReply.body = []byte("data: " + string(secondReply.body) + "\n\n")
			}
			require.True(t, t.Run("resume", func(t *testing.T) {
				body, err := json.Marshal(map[string]any{"model": p.actual, "stream": stream, "store": true, "previous_response_id": firstResponse.Id, "input": input, "tools": tools, "tool_choice": "none"})
				require.NoError(t, err)
				second := runProviderMatrix(t, p, stream, string(body), providerMatrixOptions{reply: &secondReply})
				requireProviderMatrixDispatched(t, second)
				requireProviderMatrixRoute(t, p, stream, second.got)
				var sent map[string]any
				require.NoError(t, json.Unmarshal(second.got.Body, &sent))
				require.NotContains(t, sent, "previous_response_id", "gateway selector must be resolved locally")
				contents := sent["contents"].([]any)
				require.Len(t, contents, 3)
				require.Equal(t, "Record both slots", contents[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"])
				callParts := contents[1].(map[string]any)["parts"].([]any)
				require.Len(t, callParts, 2)
				resultParts := contents[2].(map[string]any)["parts"].([]any)
				require.Len(t, resultParts, 2)
				for i, signature := range []string{"c3RvcmVkLWE=", "c3RvcmVkLWI="} {
					call := callParts[i].(map[string]any)
					require.Equal(t, signature, call["thoughtSignature"])
					require.Equal(t, map[string]any{"name": "record", "args": map[string]any{"slot": float64(i + 1)}}, call["functionCall"])
					result := resultParts[i].(map[string]any)["functionResponse"].(map[string]any)
					require.Equal(t, map[string]any{"name": "record", "response": map[string]any{"receipt": float64(i + 1)}}, result)
				}
				final := vertexLifecycleResponse(t, second, stream)
				require.Equal(t, "completed", final.Status)
				require.NotNil(t, final.Usage)
				require.Equal(t, 13, final.Usage.TotalTokens)
				finalRecord, err := store.GetResponse(context.Background(), owner, final.Id)
				require.NoError(t, err)
				require.Equal(t, firstResponse.Id, finalRecord.ParentResponseID)
				require.NotNil(t, finalRecord.Binding)
				require.Equal(t, p.actual, finalRecord.Binding.ActualModel)
			}))
		})
	}
}
