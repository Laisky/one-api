package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
)

func testMeta() *metalib.Meta {
	return &metalib.Meta{UserId: 1, TokenId: 1, ChannelId: 1}
}

func testOwner() state.OwnerScope {
	return state.OwnerScope{UserID: 1, TokenID: 1}
}

func mustEnv(t *testing.T, raw string) state.ItemEnvelope {
	t.Helper()
	env, err := state.NewItemEnvelope(json.RawMessage(raw), "test")
	require.NoError(t, err)
	return env
}

func seedResponse(t *testing.T, store state.ResponseStateStore, rec *state.ResponseStateRecord) *state.ResponseStateRecord {
	t.Helper()
	created, err := store.CreateResponse(context.Background(), rec, "")
	require.NoError(t, err)
	return created
}

// TestHydratePreviousResponseResolvesPriorContext verifies a chained request
// hydrates prior input and output so the effective transcript contains the parent
// context, not just the incremental input. Closes B02.
func TestHydratePreviousResponseResolvesPriorContext(t *testing.T) {
	store := enableStateForTest(t)

	id, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		InputItems:        []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":"remember alpha"}`)},
		OutputItems:       []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"alpha"}]}`)},
	})

	req := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &id,
		Input:              openai.ResponseAPIInput{"what did I ask you to remember?"},
	}
	hydrated, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetChatFallback)
	require.Nil(t, serr)

	converted, err := openai.ConvertResponseAPIToChatCompletionRequest(hydrated)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 3)
	require.Equal(t, "user", converted.Messages[0].Role)
	require.Equal(t, "remember alpha", converted.Messages[0].StringContent())
	require.Equal(t, "assistant", converted.Messages[1].Role)
	require.Equal(t, "alpha", converted.Messages[1].StringContent())
	require.Equal(t, "user", converted.Messages[2].Role)
	require.Equal(t, "what did I ask you to remember?", converted.Messages[2].StringContent())
}

// TestHydratePriorToolCallLink verifies that a function_call_output linked to a
// function call in the referenced parent response resolves to a proper tool
// message with the call link intact, instead of an orphan user message. Closes
// B04.
func TestHydratePriorToolCallLink(t *testing.T) {
	store := enableStateForTest(t)

	id, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		InputItems:        []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":"weather?"}`)},
		OutputItems: []state.ItemEnvelope{
			mustEnv(t, `{"type":"function_call","call_id":"call_weather","name":"get_weather","arguments":"{}"}`),
		},
	})

	req := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &id,
		Input: openai.ResponseAPIInput{
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_weather",
				"output":  `{"temperature":21}`,
			},
		},
	}
	hydrated, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetChatFallback)
	require.Nil(t, serr)

	converted, err := openai.ConvertResponseAPIToChatCompletionRequest(hydrated)
	require.NoError(t, err)

	// Expect: user "weather?", assistant with tool_calls, tool result linked to the
	// assistant tool-call ID (IDs are normalized consistently on both sides).
	var toolCallID string
	for i := range converted.Messages {
		for _, tc := range converted.Messages[i].ToolCalls {
			toolCallID = tc.Id
		}
	}
	require.NotEmpty(t, toolCallID, "hydrated parent must contribute an assistant tool call")

	var toolMsg *int
	for i := range converted.Messages {
		if converted.Messages[i].Role == "tool" {
			idx := i
			toolMsg = &idx
		}
	}
	require.NotNil(t, toolMsg, "prior tool call must produce a linked tool message, not an orphan user message")
	require.Equal(t, toolCallID, converted.Messages[*toolMsg].ToolCallId, "tool result must link to the hydrated tool call")
	require.JSONEq(t, `{"temperature":21}`, converted.Messages[*toolMsg].StringContent())
}

// TestHydrateResolvesItemReferenceAndDropsReasoning verifies that an
// item_reference resolves to the referenced stored item and a reasoning item is
// dropped rather than degrading into an empty user message. Closes B05.
func TestHydrateResolvesItemReferenceAndDropsReasoning(t *testing.T) {
	store := enableStateForTest(t)

	refEnv := mustEnv(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"resolved answer"}]}`)
	id, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		InputItems:        []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":"q"}`)},
		OutputItems:       []state.ItemEnvelope{refEnv},
	})

	req := &openai.ResponseAPIRequest{
		Model: "gpt-5",
		Input: openai.ResponseAPIInput{
			map[string]any{"type": "reasoning", "id": "rs_1", "encrypted_content": "opaque", "summary": []any{}},
			map[string]any{"type": "item_reference", "id": refEnv.GatewayItemID},
			"follow up",
		},
	}
	hydrated, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetChatFallback)
	require.Nil(t, serr)

	converted, err := openai.ConvertResponseAPIToChatCompletionRequest(hydrated)
	require.NoError(t, err)

	// No empty messages should be produced.
	for _, m := range converted.Messages {
		require.NotEmpty(t, m.StringContent(), "no hydrated message may be empty")
	}
	// The item_reference resolved to the assistant message.
	require.Equal(t, "resolved answer", converted.Messages[0].StringContent())
	require.Equal(t, "follow up", converted.Messages[len(converted.Messages)-1].StringContent())
}

// TestHydrateUnknownParentReturnsNotFound verifies an unknown/foreign parent
// returns the previous_response_not_found error without an upstream call (C04-C06,
// E03).
func TestHydrateUnknownParentReturnsNotFound(t *testing.T) {
	enableStateForTest(t)

	unknown, err := state.NewResponseID()
	require.NoError(t, err)
	req := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &unknown,
		Input:              openai.ResponseAPIInput{"hi"},
	}
	_, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetChatFallback)
	require.NotNil(t, serr)
	require.Equal(t, codePreviousResponseMissing, serr.Code)
	require.Equal(t, 400, serr.StatusCode)
}

// TestHydrateForeignOwnerParentIsNotFound verifies a parent owned by another
// tenant returns the same not-found shape as an unknown ID (C06, SEC03).
func TestHydrateForeignOwnerParentIsNotFound(t *testing.T) {
	store := enableStateForTest(t)

	id, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             state.OwnerScope{UserID: 999, TokenID: 999},
		Status:            state.StatusCompleted,
		InputItems:        []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":"secret"}`)},
	})

	req := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &id,
		Input:              openai.ResponseAPIInput{"hi"},
	}
	_, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetChatFallback)
	require.NotNil(t, serr)
	require.Equal(t, codePreviousResponseMissing, serr.Code)
}

// TestHydrateDisabledIsNoOp verifies that with the feature disabled the request
// is returned unchanged (row O01).
func TestHydrateDisabledIsNoOp(t *testing.T) {
	require.False(t, state.Enabled())
	id := "resp_whatever"
	req := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &id,
		Input:              openai.ResponseAPIInput{"hi"},
	}
	hydrated, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), req, targetChatFallback)
	require.Nil(t, serr)
	require.Same(t, req, hydrated)
}
