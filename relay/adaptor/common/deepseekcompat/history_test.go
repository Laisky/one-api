package deepseekcompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// toolCall builds an assistant tool call with the given id.
func toolCall(id string) model.Tool {
	return model.Tool{
		Id:       id,
		Type:     "function",
		Function: &model.Function{Name: "get_weather", Arguments: `{"city":"Paris"}`},
	}
}

// strPtr returns a pointer to value, for the *string message fields.
func strPtr(value string) *string {
	return &value
}

// TestEnforceHistoryContractKeepsAnsweredToolTurn pins the no-op case: a complete
// agent turn is forwarded untouched, so the repairs never disturb valid history.
func TestEnforceHistoryContractKeepsAnsweredToolTurn(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "weather in Paris?"},
		{
			Role:             "assistant",
			Content:          "Let me check.",
			ReasoningContent: strPtr("call the tool"),
			ToolCalls:        []model.Tool{toolCall("call_1")},
		},
		{Role: "tool", ToolCallId: "call_1", Content: `{"temp_c":18}`},
	}
	original := append([]model.Message(nil), messages...)

	repaired, stats := EnforceHistoryContract(messages)

	require.False(t, stats.Changed())
	require.Equal(t, original, repaired)
}

// TestEnforceHistoryContractDropsUnansweredToolCall covers the upstream rule
// "An assistant message with 'tool_calls' must be followed by tool messages
// responding to each 'tool_call_id'": the unanswered call is removed, and the
// assistant text the client actually wrote survives.
func TestEnforceHistoryContractDropsUnansweredToolCall(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "weather in Paris?"},
		{
			Role:             "assistant",
			Content:          "Checking.",
			ReasoningContent: strPtr("call the tool"),
			ToolCalls:        []model.Tool{toolCall("call_1")},
		},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 1, stats.UnansweredToolCallsDropped)
	require.Equal(t, 0, stats.AssistantMessagesDropped)
	require.Len(t, repaired, 2)
	require.Empty(t, repaired[1].ToolCalls)
	require.Equal(t, "Checking.", repaired[1].Content)
	require.Equal(t, "call the tool", *repaired[1].ReasoningContent)
}

// TestEnforceHistoryContractKeepsPartiallyAnsweredCalls guards parallel tool
// turns: only the calls nobody answered are removed, so an agent loop that lost
// one of several results keeps the results it does have.
func TestEnforceHistoryContractKeepsPartiallyAnsweredCalls(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "weather?"},
		{
			Role:      "assistant",
			Content:   "",
			ToolCalls: []model.Tool{toolCall("call_1"), toolCall("call_2")},
		},
		{Role: "tool", ToolCallId: "call_2", Content: "22"},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 1, stats.UnansweredToolCallsDropped)
	require.Len(t, repaired, 3)
	require.Len(t, repaired[1].ToolCalls, 1)
	require.Equal(t, "call_2", repaired[1].ToolCalls[0].Id)
}

// TestEnforceHistoryContractDropsEmptiedAssistantMessage covers the case where an
// assistant message carried nothing but unanswered calls: forwarding it would add
// an empty assistant turn, so the message goes away with its calls.
func TestEnforceHistoryContractDropsEmptiedAssistantMessage(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "weather?"},
		{Role: "assistant", Content: "", ToolCalls: []model.Tool{toolCall("call_1")}},
		{Role: "user", Content: "never mind"},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 1, stats.UnansweredToolCallsDropped)
	require.Equal(t, 1, stats.AssistantMessagesDropped)
	require.Len(t, repaired, 2)
	require.Equal(t, "user", repaired[0].Role)
	require.Equal(t, "never mind", repaired[1].Content)
}

// TestEnforceHistoryContractOnlyPairsAdjacentToolRun pins the Chat Completions
// pairing rule: a tool message that does not belong to the run directly following
// the assistant message cannot keep that assistant's call alive.
func TestEnforceHistoryContractOnlyPairsAdjacentToolRun(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "weather?"},
		{Role: "assistant", Content: "Checking.", ToolCalls: []model.Tool{toolCall("call_1")}},
		{Role: "user", Content: "here is the result"},
		{Role: "tool", ToolCallId: "call_1", Content: "18"},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 1, stats.UnansweredToolCallsDropped)
	require.Empty(t, repaired[1].ToolCalls)
}

// TestEnforceHistoryContractAddsReasoningToInFlightTurn covers the upstream rule
// "The `reasoning_content` in the thinking mode must be passed back to the API":
// assistant messages after the last user message must carry the field, and an
// empty string satisfies it.
func TestEnforceHistoryContractAddsReasoningToInFlightTurn(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "weather?"},
		{Role: "assistant", Content: "Checking.", ToolCalls: []model.Tool{toolCall("call_1")}},
		{Role: "tool", ToolCallId: "call_1", Content: "18"},
		{Role: "assistant", Content: "It is 18C."},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 1, stats.ReasoningPlaceholdersAdded)
	require.Nil(t, repaired[1].ReasoningContent, "a tool-call message is exempt from the rule")
	require.NotNil(t, repaired[3].ReasoningContent)
	require.Equal(t, "", *repaired[3].ReasoningContent)

	// The placeholder only helps if it reaches the wire: `omitempty` on a *string
	// keeps a pointer to "", and dropping the field is exactly what upstream rejects.
	encoded, err := json.Marshal(repaired[3])
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"reasoning_content":""`)
}

// TestEnforceHistoryContractLeavesCompletedTurnsAlone pins the rule's scope:
// upstream only validates the assistant messages after the last user message, so
// older turns keep exactly what the client replayed.
func TestEnforceHistoryContractLeavesCompletedTurnsAlone(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "more"},
		{Role: "assistant", Content: "sure"},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 1, stats.ReasoningPlaceholdersAdded)
	require.Nil(t, repaired[1].ReasoningContent)
	require.NotNil(t, repaired[3].ReasoningContent)
}

// TestEnforceHistoryContractNeverOverwritesReasoning guards the replayed thinking
// itself: the repair only fills an absent field.
func TestEnforceHistoryContractNeverOverwritesReasoning(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello", ReasoningContent: strPtr("real thinking")},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 0, stats.ReasoningPlaceholdersAdded)
	require.Equal(t, "real thinking", *repaired[1].ReasoningContent)
}

// TestEnforceHistoryContractCoversHistoryWithoutUserMessage pins the boundary when
// no user message exists: every assistant message is then part of the in-flight
// turn, which upstream validates.
func TestEnforceHistoryContractCoversHistoryWithoutUserMessage(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "system", Content: "be terse"},
		{Role: "assistant", Content: "ok"},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 1, stats.ReasoningPlaceholdersAdded)
	require.NotNil(t, repaired[1].ReasoningContent)
}

// TestEnforceHistoryContractHandlesEmptyHistory guards the trivial inputs.
func TestEnforceHistoryContractHandlesEmptyHistory(t *testing.T) {
	t.Parallel()

	repaired, stats := EnforceHistoryContract(nil)
	require.Nil(t, repaired)
	require.False(t, stats.Changed())
}

// TestEnforceHistoryContractRepairsBothRulesTogether covers the interaction that
// makes ordering matter: dropping an unanswered call turns the message into a
// plain assistant message, which then needs the reasoning field.
func TestEnforceHistoryContractRepairsBothRulesTogether(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "weather?"},
		{Role: "assistant", Content: "Checking.", ToolCalls: []model.Tool{toolCall("call_1")}},
	}

	repaired, stats := EnforceHistoryContract(messages)

	require.Equal(t, 1, stats.UnansweredToolCallsDropped)
	require.Equal(t, 1, stats.ReasoningPlaceholdersAdded)
	require.Empty(t, repaired[1].ToolCalls)
	require.NotNil(t, repaired[1].ReasoningContent)
}

// TestEnforceHistoryContractDoesNotMutateCallerSliceLength guards the copy-on-write
// behavior: the caller's backing array must not be rewritten when messages are
// dropped, because callers keep their own slice header.
func TestEnforceHistoryContractDoesNotMutateCallerSliceLength(t *testing.T) {
	t.Parallel()

	messages := []model.Message{
		{Role: "user", Content: "weather?"},
		{Role: "assistant", Content: "", ToolCalls: []model.Tool{toolCall("call_1")}},
		{Role: "user", Content: "never mind"},
	}

	repaired, _ := EnforceHistoryContract(messages)

	require.Len(t, repaired, 2)
	require.Len(t, messages, 3, "the caller's slice header must still describe the original history")
	require.Equal(t, "assistant", messages[1].Role)
	require.Equal(t, "never mind", messages[2].Content)
}
