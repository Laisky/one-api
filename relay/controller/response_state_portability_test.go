package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// TestHydrateHostedToolStateNotPortable verifies that a hydrated chain whose prior
// output carries provider-hosted tool-call state fails closed with
// state_not_portable on a stateless fallback route rather than silently dropping
// it (ST-008: P04, E05).
func TestHydrateHostedToolStateNotPortable(t *testing.T) {
	store := enableStateForTest(t)

	id, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		InputItems:        []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":"search please"}`)},
		OutputItems: []state.ItemEnvelope{
			mustEnv(t, `{"type":"code_interpreter_call","id":"ci_1","status":"completed"}`),
		},
	})

	req := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &id,
		Input:              openai.ResponseAPIInput{"and then?"},
	}
	_, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetChatFallback)
	require.NotNil(t, serr)
	require.Equal(t, codeStateNotPortable, serr.Code)
	require.Equal(t, http.StatusConflict, serr.StatusCode)
}

// TestHydrateWebSearchCallReachesFallbackConverter verifies that a hydrated
// chain whose prior output carries web_search_call items keeps them through
// hydration on every fallback target, so the converter can see that the stored
// Claude response ran provider searches: it then omits that response's thinking
// (Claude rejects replayed thinking whose server-tool blocks are missing) and
// annotates the answer with the searches, instead of replaying the envelope.
func TestHydrateWebSearchCallReachesFallbackConverter(t *testing.T) {
	store := enableStateForTest(t)
	envelope, err := relaymodel.EncodeClaudeThinkingEnvelope(relaymodel.ClaudeThinkingBlock{Type: "thinking", Signature: "sig_search"})
	require.NoError(t, err)

	id, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		InputItems:        []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":"search please"}`)},
		OutputItems: []state.ItemEnvelope{
			mustEnv(t, `{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"`+envelope+`"}`),
			mustEnv(t, `{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q"}}`),
			mustEnv(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"found it"}]}`),
		},
	})

	for _, target := range []hydrationTarget{targetChatFallback, targetClaudeFallback} {
		req := &openai.ResponseAPIRequest{
			Model:              "claude-opus-5-5",
			PreviousResponseId: &id,
			Input:              openai.ResponseAPIInput{"and then?"},
		}
		hydrated, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, target)
		require.Nil(t, serr)
		searches := 0
		for _, item := range hydrated.Input {
			if fields, ok := item.(map[string]any); ok && fields["type"] == "web_search_call" {
				searches++
			}
		}
		require.Equal(t, 1, searches, "web_search_call must reach the converter on target %v", target)

		converted, err := openai.ConvertResponseAPIToChatCompletionRequest(hydrated)
		require.NoError(t, err)
		var assistant *relaymodel.Message
		for i := range converted.Messages {
			if converted.Messages[i].Role == "assistant" {
				assistant = &converted.Messages[i]
			}
		}
		require.NotNil(t, assistant)
		require.Empty(t, assistant.ClaudeThinkingBlocks, "thinking of a searched response must not be replayed")
		require.Equal(t, "[Web search: q]\nfound it", assistant.StringContent())
		require.Equal(t, "and then?", converted.Messages[len(converted.Messages)-1].StringContent())
	}
}

// TestHydrateUnknownItemTypeNotPortable verifies an unknown future Responses item
// type replayed in the current input fails closed on fallback (ST-008: I05).
func TestHydrateUnknownItemTypeNotPortable(t *testing.T) {
	enableStateForTest(t)

	req := &openai.ResponseAPIRequest{
		Model: "gpt-5",
		Input: openai.ResponseAPIInput{
			map[string]any{"type": "some_future_item", "id": "x"},
			"hello",
		},
	}
	_, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetClaudeFallback)
	require.NotNil(t, serr)
	require.Equal(t, codeStateNotPortable, serr.Code)
}

// TestHydrateReasoningStillDropsOnClaudeTarget verifies reasoning degrades to a
// sidecar (drop) on a Claude fallback target too, without erroring.
func TestHydrateReasoningStillDropsOnClaudeTarget(t *testing.T) {
	enableStateForTest(t)

	req := &openai.ResponseAPIRequest{
		Model: "gpt-5",
		Input: openai.ResponseAPIInput{
			map[string]any{"type": "reasoning", "id": "rs_1", "encrypted_content": "opaque", "summary": []any{}},
			"follow up",
		},
	}
	hydrated, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetClaudeFallback)
	require.Nil(t, serr)
	require.Len(t, hydrated.Input, 1) // reasoning dropped, only the user text remains
}

// TestHydratePriorItemReferenceResolves verifies that an item_reference stored in
// a PARENT turn's input (hydrated prior context, not the current input) is
// resolved before lowering rather than degrading into an empty message (review
// finding fix; P05/I04).
func TestHydratePriorItemReferenceResolves(t *testing.T) {
	store := enableStateForTest(t)

	// refEnv is a stored assistant message that a prior turn referenced by id.
	refEnv := mustEnv(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"resolved prior answer"}]}`)

	parentID, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: parentID,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		// The parent's INCREMENTAL INPUT contains an unresolved item_reference; refEnv
		// is retrievable because it is stored as this node's output.
		InputItems: []state.ItemEnvelope{
			mustEnv(t, `{"type":"item_reference","id":"`+refEnv.GatewayItemID+`"}`),
		},
		OutputItems: []state.ItemEnvelope{refEnv},
	})

	req := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &parentID,
		Input:              openai.ResponseAPIInput{"and now?"},
	}
	hydrated, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetChatFallback)
	require.Nil(t, serr)

	converted, err := openai.ConvertResponseAPIToChatCompletionRequest(hydrated)
	require.NoError(t, err)
	for _, m := range converted.Messages {
		require.NotEmpty(t, m.StringContent(), "no hydrated message may be empty (prior item_reference must resolve)")
	}
	// The resolved reference content is present in the effective transcript.
	found := false
	for _, m := range converted.Messages {
		if m.StringContent() == "resolved prior answer" {
			found = true
		}
	}
	require.True(t, found, "prior item_reference must resolve to its stored content")
}

// TestResponseFallbackTarget maps upstream api types to the lowering target.
func TestResponseFallbackTarget(t *testing.T) {
	require.Equal(t, targetChatFallback, responseFallbackTarget(&metalib.Meta{APIType: apitype.OpenAI}))
	require.Equal(t, targetClaudeFallback, responseFallbackTarget(&metalib.Meta{APIType: apitype.Anthropic}))
	require.Equal(t, targetClaudeFallback, responseFallbackTarget(&metalib.Meta{APIType: apitype.AwsClaude}))
	require.Equal(t, targetClaudeFallback, responseFallbackTarget(&metalib.Meta{APIType: apitype.VertexAI, ActualModelName: "claude-sonnet-5"}))
	require.Equal(t, targetChatFallback, responseFallbackTarget(&metalib.Meta{APIType: apitype.VertexAI, ActualModelName: "gemini-2.5-pro"}))
}
