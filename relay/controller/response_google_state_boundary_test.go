package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	vertexgemini "github.com/Laisky/one-api/relay/adaptor/vertexai/gemini"
	"github.com/Laisky/one-api/relay/apitype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/state"
)

// TestStoredGoogleMetadataBoundary probes opaque metadata portability across mapped targets.
func TestStoredGoogleMetadataBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		api         int
		carry       bool
	}{
		{"same_model_provider", "gemini-3.8-flash", apitype.VertexAI, true},
		{"different_model", "gemini-2.5-flash", apitype.VertexAI, false},
		{"different_api", "gemini-3.8-flash", apitype.Gemini, false},
		{"non_google", "gpt-review", apitype.OpenAI, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := enableStateForTest(t)
			id, err := state.NewResponseID()
			require.NoError(t, err)
			seedResponse(t, store, &state.ResponseStateRecord{GatewayResponseID: id, Owner: testOwner(), Status: state.StatusCompleted,
				Binding:     &state.ProviderBinding{ChannelID: 1, APIType: apitype.VertexAI, ActualModel: "gemini-3.8-flash"},
				OutputItems: []state.ItemEnvelope{mustEnv(t, `{"type":"function_call","id":"call_boundary","call_id":"call_boundary","name":"record","arguments":"{}","extra_content":{"google":{"thought_signature":"origin-model-opaque"},"oneapi":{"gemini_part_index":0}}}`)},
			})
			m := testMeta()
			m.APIType = tc.api
			m.ActualModelName = tc.model
			m.OriginModelName = tc.model
			req := &openai.ResponseAPIRequest{Model: tc.model, PreviousResponseId: &id, Input: openai.ResponseAPIInput{map[string]any{"type": "function_call_output", "call_id": "call_boundary", "output": "done"}}}
			hydrated, apiErr := hydrateResponseAPIRequestForFallback(context.Background(), m, req, targetChatFallback)
			if tc.api != apitype.OpenAI && !tc.carry {
				require.NotNil(t, apiErr)
				require.Equal(t, codeStateNotPortable, apiErr.Code)
				return
			}
			require.Nil(t, apiErr)
			chat, err := openai.ConvertResponseAPIToChatCompletionRequestWithOptions(hydrated, nil, responseFallbackConversionOptions(m))
			require.NoError(t, err)
			raw, err := json.Marshal(chat)
			require.NoError(t, err)
			if tc.api == apitype.VertexAI {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				metalib.Set2Context(c, m)
				native, nativeErr := (&vertexgemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, chat)
				require.NoError(t, nativeErr)
				raw, err = json.Marshal(native)
				require.NoError(t, err)
			}
			if !tc.carry {
				require.NotContains(t, string(raw), "origin-model-opaque", "provider-bound signed state must not replay across a changed target")
				return
			}
			require.Contains(t, string(raw), "origin-model-opaque")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			metalib.Set2Context(c, m)
			native, err := (&vertexgemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, chat)
			require.NoError(t, err)
			raw, err = json.Marshal(native)
			require.NoError(t, err)
			require.Contains(t, string(raw), `"thoughtSignature":"origin-model-opaque"`)
		})
	}
}

// TestStoredGoogleMetadataPortabilityControls keeps unsigned canonical state portable without optional acceleration bindings.
func TestStoredGoogleMetadataPortabilityControls(t *testing.T) {
	target := testMeta()
	target.APIType = apitype.VertexAI
	target.ActualModelName = "gemini-2.5-flash"
	for _, raw := range []string{
		`{"type":"function_call","call_id":"a","name":"record","arguments":"{}","extra_content":{"oneapi":{"gemini_part_index":0}}}`,
		`{"type":"function_call","call_id":"a","name":"record","arguments":"{\"extra_content\":{\"google\":{\"thought_signature\":\"literal\"}}}"}`,
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"extra_content google thought_signature literal"}]}`,
	} {
		require.Nil(t, validateStoredGoogleMetadata([]state.ItemEnvelope{mustEnv(t, raw)}, nil, target))
	}
	signed := mustEnv(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer","extra_content":{"google":{"thought_signature":"signed"}}}]}`)
	require.NotNil(t, validateStoredGoogleMetadata([]state.ItemEnvelope{signed}, nil, target))
	binding := &state.ProviderBinding{ChannelID: target.ChannelId, APIType: target.APIType, ActualModel: target.ActualModelName}
	require.Nil(t, validateStoredGoogleMetadata([]state.ItemEnvelope{signed}, binding, target))
	binding.ChannelID++
	require.NotNil(t, validateStoredGoogleMetadata([]state.ItemEnvelope{signed}, binding, target))
}

// TestStoredGoogleConversationAndReferences covers bounded stored selectors without adding a storage schema.
func TestStoredGoogleConversationAndReferences(t *testing.T) {
	for _, selector := range []string{"conversation_same", "conversation_other", "item_reference"} {
		t.Run(selector, func(t *testing.T) {
			store := enableStateForTest(t)
			target := testMeta()
			target.APIType = apitype.VertexAI
			target.ActualModelName = "gemini-3.8-flash"
			signed := mustEnv(t, `{"type":"function_call","id":"call_ref","call_id":"call_ref","name":"record","arguments":"{}","extra_content":{"google":{"thought_signature":"opaque"},"oneapi":{"gemini_part_index":0}}}`)
			request := &openai.ResponseAPIRequest{Model: target.ActualModelName, Input: openai.ResponseAPIInput{map[string]any{"type": "function_call_output", "call_id": "call_ref", "output": "done"}}}
			if selector == "item_reference" {
				id, err := state.NewResponseID()
				require.NoError(t, err)
				seedResponse(t, store, &state.ResponseStateRecord{GatewayResponseID: id, Owner: testOwner(), Status: state.StatusCompleted, OutputItems: []state.ItemEnvelope{signed}})
				request.Input = append(openai.ResponseAPIInput{map[string]any{"type": "item_reference", "id": signed.GatewayItemID}}, request.Input...)
			} else {
				id, err := state.NewConversationID()
				require.NoError(t, err)
				binding := &state.ProviderBinding{ChannelID: target.ChannelId, APIType: target.APIType, ActualModel: target.ActualModelName}
				if selector == "conversation_other" {
					binding.ActualModel = "gemini-2.5-flash"
				}
				_, err = store.CreateConversation(context.Background(), &state.ConversationStateRecord{GatewayConversationID: id, Owner: testOwner(), Items: []state.ItemEnvelope{signed}, Binding: binding}, "")
				require.NoError(t, err)
				request.Conversation = &openai.ResponseAPIConversation{Id: id}
			}
			_, apiErr := hydrateResponseAPIRequestForFallback(context.Background(), target, request, targetChatFallback)
			if selector == "conversation_same" {
				require.Nil(t, apiErr)
			} else {
				require.NotNil(t, apiErr)
				require.Equal(t, codeStateNotPortable, apiErr.Code)
			}
		})
	}
}

// TestStoredGoogleAncestorBoundary checks every stored response before its binding provenance is flattened.
func TestStoredGoogleAncestorBoundary(t *testing.T) {
	store := enableStateForTest(t)
	target := testMeta()
	target.APIType = apitype.VertexAI
	target.ActualModelName = "gemini-2.5-flash"
	parent, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{GatewayResponseID: parent, Owner: testOwner(), Status: state.StatusCompleted, Binding: &state.ProviderBinding{ChannelID: target.ChannelId, APIType: apitype.VertexAI, ActualModel: "gemini-3.8-flash"}, OutputItems: []state.ItemEnvelope{mustEnv(t, `{"type":"function_call","id":"call_old","call_id":"call_old","name":"record","arguments":"{}","extra_content":{"google":{"thought_signature":"ancestor"}}}`)}})
	head, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{GatewayResponseID: head, Owner: testOwner(), Status: state.StatusCompleted, ParentResponseID: parent, Binding: &state.ProviderBinding{ChannelID: target.ChannelId, APIType: target.APIType, ActualModel: target.ActualModelName}, OutputItems: []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"plain"}]}`)}})
	_, apiErr := hydrateResponseAPIRequestForFallback(context.Background(), target, &openai.ResponseAPIRequest{Model: target.ActualModelName, PreviousResponseId: &head, Input: openai.ResponseAPIInput{"continue"}}, targetChatFallback)
	require.NotNil(t, apiErr)
	require.Equal(t, codeStateNotPortable, apiErr.Code)
}

// TestStoredGoogleBoundaryBeforeDispatch proves signed state rejects before transport while unsigned history remains portable.
func TestStoredGoogleBoundaryBeforeDispatch(t *testing.T) {
	for _, signed := range []bool{false, true} {
		t.Run(fmt.Sprint(signed), func(t *testing.T) {
			enableStateForTest(t)
			p := vertexLifecycleProvider()
			var saved openai.ResponseAPIResponse
			signature := ""
			if signed {
				signature = `,"thoughtSignature":"bound-opaque"`
			}
			reply := providerMatrixReply{contentType: "application/json", body: []byte(`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"functionCall":{"name":"record","args":{}}` + signature + `}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}`)}
			require.True(t, t.Run("store", func(t *testing.T) {
				result := runProviderMatrix(t, p, false, `{"model":"gemini-3.8-flash","store":true,"input":"record","tools":[{"type":"function","name":"record","parameters":{"type":"object"}}]}`, providerMatrixOptions{reply: &reply})
				requireProviderMatrixDispatched(t, result)
				saved = vertexLifecycleResponse(t, result, false)
				require.Len(t, saved.Output, 1)
			}))
			require.True(t, t.Run("changed_model", func(t *testing.T) {
				p.actual = "gemini-2.5-flash"
				body, err := json.Marshal(map[string]any{"model": p.actual, "previous_response_id": saved.Id, "input": []any{map[string]any{"type": "function_call_output", "call_id": saved.Output[0].CallId, "output": "done"}}})
				require.NoError(t, err)
				result := runProviderMatrix(t, p, false, string(body), providerMatrixOptions{})
				if signed {
					require.NotNil(t, result.apiErr)
					require.Equal(t, 400, result.apiErr.StatusCode)
					require.Equal(t, codeStateNotPortable, result.apiErr.Code)
					require.Zero(t, result.calls)
					require.False(t, result.observed)
				} else {
					requireProviderMatrixDispatched(t, result)
					require.NotContains(t, string(result.got.Body), "thoughtSignature")
				}
			}))
		})
	}
}
