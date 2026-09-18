package deepseek

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// newDeepSeekTestContext returns a gin context usable by the conversion helpers.
func newDeepSeekTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	return c
}

// TestConvertRequestDropsUnansweredToolCalls covers history whose tool calls lost
// their results, for example because a client trimmed the conversation or replayed
// a call_id that no longer matches. DeepSeek rejects it with "An assistant message
// with 'tool_calls' must be followed by tool messages responding to each
// 'tool_call_id'" (verified against api.deepseek.com with deepseek-flash on
// 2026-09-18), so the adaptor drops the unanswered call instead of forwarding a
// request that cannot succeed.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when the invalid call is forwarded.
func TestConvertRequestDropsUnansweredToolCalls(t *testing.T) {
	t.Parallel()

	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-flash",
		Messages: []model.Message{
			{Role: "user", Content: "weather in Paris?"},
			{
				Role:    "assistant",
				Content: "Checking.",
				ToolCalls: []model.Tool{{
					Id:       "call_1",
					Type:     "function",
					Function: &model.Function{Name: "get_weather", Arguments: `{"city":"Paris"}`},
				}},
			},
		},
	}

	convertedAny, err := (&Adaptor{}).ConvertRequest(newDeepSeekTestContext(), 0, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)

	require.Len(t, converted.Messages, 2)
	require.Empty(t, converted.Messages[1].ToolCalls)
	require.Equal(t, "Checking.", converted.Messages[1].Content)
}

// TestConvertRequestReplaysThinkingFieldOnInFlightTurn covers the other rule the
// adaptor has to satisfy: every assistant message after the last user message that
// carries no tool_calls must send a reasoning_content field, or DeepSeek answers
// "The `reasoning_content` in the thinking mode must be passed back to the API".
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when the field is missing.
func TestConvertRequestReplaysThinkingFieldOnInFlightTurn(t *testing.T) {
	t.Parallel()

	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-flash",
		Messages: []model.Message{
			{Role: "user", Content: "weather in Paris?"},
			{Role: "assistant", Content: "It is 18C."},
		},
	}

	convertedAny, err := (&Adaptor{}).ConvertRequest(newDeepSeekTestContext(), 0, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)

	require.Len(t, converted.Messages, 2)
	require.NotNil(t, converted.Messages[1].ReasoningContent)
	require.Equal(t, "", *converted.Messages[1].ReasoningContent)
}

// TestConvertRequestKeepsValidToolTurnIntact guards the no-op path: a complete
// agent turn must reach upstream exactly as the client sent it.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when valid history is rewritten.
func TestConvertRequestKeepsValidToolTurnIntact(t *testing.T) {
	t.Parallel()

	reasoning := "call the tool"
	request := &model.GeneralOpenAIRequest{
		Model: "deepseek-flash",
		Messages: []model.Message{
			{Role: "user", Content: "weather in Paris?"},
			{
				Role:             "assistant",
				Content:          "Checking.",
				ReasoningContent: &reasoning,
				ToolCalls: []model.Tool{{
					Id:       "call_1",
					Type:     "function",
					Function: &model.Function{Name: "get_weather", Arguments: `{"city":"Paris"}`},
				}},
			},
			{Role: "tool", ToolCallId: "call_1", Content: `{"temp_c":18}`},
		},
	}

	convertedAny, err := (&Adaptor{}).ConvertRequest(newDeepSeekTestContext(), 0, request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)

	require.Len(t, converted.Messages, 3)
	require.Len(t, converted.Messages[1].ToolCalls, 1)
	require.Equal(t, "call_1", converted.Messages[1].ToolCalls[0].Id)
	require.Equal(t, reasoning, *converted.Messages[1].ReasoningContent)
	require.Equal(t, "tool", converted.Messages[2].Role)
}

// TestConvertClaudeRequestEnforcesHistoryContract covers the Claude Messages entry
// point, which reaches the same upstream endpoint and therefore the same rules.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when the contract is not applied.
func TestConvertClaudeRequestEnforcesHistoryContract(t *testing.T) {
	t.Parallel()

	request := &model.ClaudeRequest{
		Model:     "deepseek-flash",
		MaxTokens: 1024,
		Messages: []model.ClaudeMessage{
			{Role: "user", Content: "weather in Paris?"},
			{Role: "assistant", Content: "It is 18C."},
		},
	}

	convertedAny, err := (&Adaptor{}).ConvertClaudeRequest(newDeepSeekTestContext(), request)
	require.NoError(t, err)
	converted, ok := convertedAny.(*model.GeneralOpenAIRequest)
	require.True(t, ok)

	last := converted.Messages[len(converted.Messages)-1]
	require.Equal(t, "assistant", last.Role)
	require.NotNil(t, last.ReasoningContent)
}
