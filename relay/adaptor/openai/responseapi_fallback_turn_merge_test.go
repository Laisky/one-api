package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// fallbackTurnTools declares the Codex collaboration namespace whose
// spawn_agent/wait_agent calls appear in multi-agent history.
const fallbackTurnTools = `[{"type":"namespace","name":"collaboration","description":"Multi-agent collaboration","tools":[
	{"type":"function","name":"spawn_agent","parameters":{"type":"object","properties":{"task":{"type":"string"}}}},
	{"type":"function","name":"wait_agent","parameters":{"type":"object","properties":{"id":{"type":"string"}}}}
]}]`

// fallbackTurnCallA and fallbackTurnCallB are two parallel namespaced calls of one assistant turn.
const (
	fallbackTurnCallA = `{"type":"function_call","id":"call_a","call_id":"call_a","namespace":"collaboration","name":"spawn_agent","arguments":"{\"task\":\"review\"}"}`
	fallbackTurnCallB = `{"type":"function_call","id":"call_b","call_id":"call_b","namespace":"collaboration","name":"wait_agent","arguments":"{\"id\":\"w1\"}"}`
	fallbackTurnOutA  = `{"type":"function_call_output","call_id":"call_a","output":"spawned w1"}`
	fallbackTurnOutB  = `{"type":"function_call_output","call_id":"call_b","output":"w1 finished"}`
)

// fallbackTurnAssistantText builds an assistant message item carrying text.
// Parameters: text is the output_text payload. Returns the item JSON.
func fallbackTurnAssistantText(text string) string {
	return `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + text + `"}]}`
}

// fallbackTurnConvert converts a request whose input is items, using the
// collaboration namespace tools. Parameters: t owns assertions and items is
// the comma-joined input item JSON. Returns the converted Chat messages.
func fallbackTurnConvert(t *testing.T, items string) []model.Message {
	t.Helper()
	request := fallbackToolRequest(t, `{"model":"fixture","tools":`+fallbackTurnTools+`,"input":[{"type":"message","role":"user","content":"coordinate"},`+items+`]}`)
	converted, err := ConvertResponseAPIToChatCompletionRequest(request)
	require.NoError(t, err)
	return converted.Messages
}

// fallbackTurnRequireCalls asserts message holds exactly the given flattened
// tool-call names in order. Parameters: t owns assertions, message is the
// assistant message and names are the expected flattened function names.
func fallbackTurnRequireCalls(t *testing.T, message model.Message, names ...string) {
	t.Helper()
	require.Equal(t, "assistant", message.Role)
	require.Len(t, message.ToolCalls, len(names))
	for i, name := range names {
		require.NotNil(t, message.ToolCalls[i].Function)
		require.Equal(t, name, message.ToolCalls[i].Function.Name, "tool call %d", i)
	}
}

// TestConvertResponseAPIToChatCompletionRequestMergesAssistantTextIntoOpenTurn
// proves the Codex history shape [call A, call B, assistant text, output A,
// output B] lowers to ONE assistant message holding the text and both calls,
// followed by two tool messages paired to A and B, wherever the text sits
// between the calls and their outputs.
func TestConvertResponseAPIToChatCompletionRequestMergesAssistantTextIntoOpenTurn(t *testing.T) {
	t.Parallel()
	text := fallbackTurnAssistantText("Spawning a reviewer.")
	for name, items := range map[string]string{
		"text_after_calls":   fallbackTurnCallA + "," + fallbackTurnCallB + "," + text + "," + fallbackTurnOutA + "," + fallbackTurnOutB,
		"text_between_calls": fallbackTurnCallA + "," + text + "," + fallbackTurnCallB + "," + fallbackTurnOutA + "," + fallbackTurnOutB,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			messages := fallbackTurnConvert(t, items)
			require.Len(t, messages, 4, "user, one merged assistant turn, two tool results")
			require.Equal(t, "user", messages[0].Role)
			fallbackTurnRequireCalls(t, messages[1], "collaboration__spawn_agent", "collaboration__wait_agent")
			require.Equal(t, "Spawning a reviewer.", messages[1].StringContent())
			require.Equal(t, "tool", messages[2].Role)
			require.Equal(t, messages[1].ToolCalls[0].Id, messages[2].ToolCallId)
			require.Equal(t, "spawned w1", messages[2].StringContent())
			require.Equal(t, "tool", messages[3].Role)
			require.Equal(t, messages[1].ToolCalls[1].Id, messages[3].ToolCallId)
			require.Equal(t, "w1 finished", messages[3].StringContent())
		})
	}
}

// TestConvertResponseAPIToChatCompletionRequestMergeJoinsTextAndReasoning
// proves a merged assistant item appends to existing turn text with a newline
// and attaches its own reasoning when the turn has none yet.
func TestConvertResponseAPIToChatCompletionRequestMergeJoinsTextAndReasoning(t *testing.T) {
	t.Parallel()
	items := fallbackTurnAssistantText("First.") + "," + fallbackTurnCallA + "," + fallbackTurnAssistantText("Second.") + "," + fallbackTurnOutA
	messages := fallbackTurnConvert(t, items)
	require.Len(t, messages, 3)
	fallbackTurnRequireCalls(t, messages[1], "collaboration__spawn_agent")
	require.Equal(t, "First.\nSecond.", messages[1].StringContent())
	require.Equal(t, "tool", messages[2].Role)
	require.Equal(t, messages[1].ToolCalls[0].Id, messages[2].ToolCallId)

	reasoningFirst := `{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"plan the review"}]},` +
		fallbackTurnCallA + "," + fallbackTurnAssistantText("Spawning.") + "," + fallbackTurnOutA
	messages = fallbackTurnConvert(t, reasoningFirst)
	require.Len(t, messages, 3)
	fallbackTurnRequireCalls(t, messages[1], "collaboration__spawn_agent")
	require.Equal(t, "Spawning.", messages[1].StringContent())
	require.NotNil(t, messages[1].ReasoningContent, "staged reasoning stays on the merged turn")
	require.Equal(t, "plan the review", *messages[1].ReasoningContent)
}

// TestConvertResponseAPIToChatCompletionRequestMergeOnlyForOpenAssistantTurn
// proves the merge is limited to assistant text while calls are unanswered:
// a user message still closes the turn (its outputs downgrade to user text),
// and assistant text after every call was answered stays a separate message.
func TestConvertResponseAPIToChatCompletionRequestMergeOnlyForOpenAssistantTurn(t *testing.T) {
	t.Parallel()
	userBetween := fallbackTurnCallA + `,{"type":"message","role":"user","content":"interrupt"},` + fallbackTurnOutA
	messages := fallbackTurnConvert(t, userBetween)
	require.Len(t, messages, 4)
	fallbackTurnRequireCalls(t, messages[1], "collaboration__spawn_agent")
	require.Empty(t, messages[1].StringContent())
	require.Equal(t, "user", messages[2].Role)
	require.Equal(t, "interrupt", messages[2].StringContent())
	require.Equal(t, "user", messages[3].Role, "an output after a user message is an orphan")
	require.Equal(t, "spawned w1", messages[3].StringContent())

	answered := fallbackTurnCallA + "," + fallbackTurnOutA + "," + fallbackTurnAssistantText("Reviewer spawned.")
	messages = fallbackTurnConvert(t, answered)
	require.Len(t, messages, 4)
	fallbackTurnRequireCalls(t, messages[1], "collaboration__spawn_agent")
	require.Empty(t, messages[1].StringContent())
	require.Equal(t, "tool", messages[2].Role)
	require.Equal(t, "assistant", messages[3].Role)
	require.Empty(t, messages[3].ToolCalls)
	require.Equal(t, "Reviewer spawned.", messages[3].StringContent())
}

// TestConvertResponseAPIToChatCompletionRequestEmptyOrphanOutput proves an
// orphan tool output with no text becomes the nonempty marker user message
// that Claude accepts, a nonempty orphan keeps its text, and an empty output
// that answers a pending call stays a paired tool message.
func TestConvertResponseAPIToChatCompletionRequestEmptyOrphanOutput(t *testing.T) {
	t.Parallel()
	messages := fallbackTurnConvert(t, `{"type":"function_call_output","call_id":"call_orphan","output":""}`)
	require.Len(t, messages, 2)
	require.Equal(t, "user", messages[1].Role)
	require.Equal(t, "(empty tool output)", messages[1].StringContent())
	require.Equal(t, responseEmptyToolOutputText, messages[1].StringContent())

	messages = fallbackTurnConvert(t, `{"type":"function_call_output","call_id":"call_orphan","output":"late result"}`)
	require.Len(t, messages, 2)
	require.Equal(t, "user", messages[1].Role)
	require.Equal(t, "late result", messages[1].StringContent())

	messages = fallbackTurnConvert(t, fallbackTurnCallA+`,{"type":"function_call_output","call_id":"call_a","output":""}`)
	require.Len(t, messages, 3)
	fallbackTurnRequireCalls(t, messages[1], "collaboration__spawn_agent")
	require.Equal(t, "tool", messages[2].Role)
	require.Equal(t, messages[1].ToolCalls[0].Id, messages[2].ToolCallId)
	require.NotEqual(t, responseEmptyToolOutputText, messages[2].StringContent(), "paired outputs keep their own content")
}
