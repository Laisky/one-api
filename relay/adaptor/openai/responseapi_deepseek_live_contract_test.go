package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/common/deepseekcompat"
	"github.com/Laisky/one-api/relay/model"
)

// DeepSeek validates whole-conversation structure before it looks at the prompt,
// and the Responses -> Chat fallback is the only path that has to *reconstruct*
// that structure from Responses items. These tests replay realistic agent-loop
// histories through the real conversion pipeline and send the result to the live
// endpoint, which is the only way to keep the offline regression tests honest
// about what upstream actually accepts.
//
// They need a funded DeepSeek key and are therefore skipped unless
// ONEAPI_DEEPSEEK_LIVE_KEY is set:
//
//	ONEAPI_DEEPSEEK_LIVE_KEY=sk-... go test ./relay/adaptor/openai/ -run DeepSeekLive -v
//
// ONEAPI_DEEPSEEK_LIVE_BASE_URL and ONEAPI_DEEPSEEK_LIVE_MODEL override the
// defaults below. Each case sends a few hundred tokens.
const (
	deepSeekLiveKeyEnv     = "ONEAPI_DEEPSEEK_LIVE_KEY"
	deepSeekLiveBaseURLEnv = "ONEAPI_DEEPSEEK_LIVE_BASE_URL"
	deepSeekLiveModelEnv   = "ONEAPI_DEEPSEEK_LIVE_MODEL"

	deepSeekLiveDefaultBaseURL = "https://api.deepseek.com"
	deepSeekLiveDefaultModel   = "deepseek-flash"
)

// deepSeekLiveConfig holds the resolved upstream coordinates for a live run.
type deepSeekLiveConfig struct {
	key     string
	baseURL string
	model   string
}

// requireDeepSeekLiveConfig resolves the live-run settings or skips the test.
//
// Parameters:
//   - t: the testing handle; the test is skipped when no key is configured.
//
// Returns:
//   - deepSeekLiveConfig: the upstream key, base URL and model to exercise.
func requireDeepSeekLiveConfig(t *testing.T) deepSeekLiveConfig {
	t.Helper()

	key := os.Getenv(deepSeekLiveKeyEnv)
	if key == "" {
		t.Skipf("set %s to run the live DeepSeek contract tests", deepSeekLiveKeyEnv)
	}

	cfg := deepSeekLiveConfig{
		key:     key,
		baseURL: os.Getenv(deepSeekLiveBaseURLEnv),
		model:   os.Getenv(deepSeekLiveModelEnv),
	}
	if cfg.baseURL == "" {
		cfg.baseURL = deepSeekLiveDefaultBaseURL
	}
	if cfg.model == "" {
		cfg.model = deepSeekLiveDefaultModel
	}
	return cfg
}

// postDeepSeekChatCompletion sends messages to the live endpoint in thinking mode.
//
// Parameters:
//   - t: the testing handle used for fatal transport failures.
//   - cfg: the resolved upstream coordinates.
//   - messages: the chat history to validate upstream.
//
// Returns:
//   - int: the HTTP status code.
//   - string: the upstream error message, empty when the request succeeded.
func postDeepSeekChatCompletion(t *testing.T, cfg deepSeekLiveConfig, messages []model.Message) (int, string) {
	t.Helper()

	payload := map[string]any{
		"model": cfg.model,
		// Thinking mode is DeepSeek's default and is what activates the
		// reasoning_content replay rule, so state it explicitly.
		"thinking":   map[string]any{"type": "enabled"},
		"max_tokens": 64,
		"messages":   messages,
		"tools": []any{map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get weather for a city",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
					"required":   []string{"city"},
				},
			},
		}},
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.key)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck // response body close on a test request

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var decoded struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &decoded); err == nil && decoded.Error != nil {
		return resp.StatusCode, decoded.Error.Message
	}
	return resp.StatusCode, ""
}

// deepSeekLiveResponseItem builders keep the scenarios below readable.

// deepSeekLiveUserItem builds a Responses user message item.
func deepSeekLiveUserItem(text string) map[string]any {
	return map[string]any{"type": "message", "role": "user",
		"content": []any{map[string]any{"type": "input_text", "text": text}}}
}

// deepSeekLiveAssistantItem builds a Responses assistant message item.
func deepSeekLiveAssistantItem(text string) map[string]any {
	return map[string]any{"type": "message", "role": "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": text}}}
}

// deepSeekLiveReasoningItem builds a Responses reasoning item.
func deepSeekLiveReasoningItem(text string) map[string]any {
	return map[string]any{"type": "reasoning", "status": "completed",
		"content": []any{map[string]any{"type": "text", "text": text}},
		"summary": []any{map[string]any{"type": "summary_text", "text": text}}}
}

// deepSeekLiveFunctionCallItem builds a Responses function_call item.
func deepSeekLiveFunctionCallItem(callID, city string) map[string]any {
	return map[string]any{"type": "function_call", "id": callID, "call_id": callID,
		"name": "get_weather", "arguments": `{"city":"` + city + `"}`, "status": "completed"}
}

// deepSeekLiveFunctionOutputItem builds a Responses function_call_output item.
func deepSeekLiveFunctionOutputItem(callID, output string) map[string]any {
	return map[string]any{"type": "function_call_output", "call_id": callID, "output": output}
}

// TestDeepSeekLiveResponseFallbackAccepted sends every realistic Responses replay
// through the production conversion path and asserts the live endpoint accepts the
// result. Each scenario is one that upstream rejected before the fixes: the
// scenario names match the failure they used to produce.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t on any upstream rejection.
func TestDeepSeekLiveResponseFallbackAccepted(t *testing.T) {
	cfg := requireDeepSeekLiveConfig(t)

	scenarios := map[string]ResponseAPIInput{
		// The bridge emits message, reasoning, function_call in that order, so the
		// turn's thinking has to survive an item that arrives after its message.
		"tool_turn_gateway_item_order": {
			deepSeekLiveUserItem("What is the weather in Paris?"),
			deepSeekLiveAssistantItem("Let me check."),
			deepSeekLiveReasoningItem("Call the weather tool."),
			deepSeekLiveFunctionCallItem("call_1", "Paris"),
			deepSeekLiveFunctionOutputItem("call_1", `{"temp_c":18}`),
		},
		// OpenAI emits the reasoning item first; both orders must converge.
		"tool_turn_openai_item_order": {
			deepSeekLiveUserItem("What is the weather in Paris?"),
			deepSeekLiveReasoningItem("Call the weather tool."),
			deepSeekLiveAssistantItem("Let me check."),
			deepSeekLiveFunctionCallItem("call_1", "Paris"),
			deepSeekLiveFunctionOutputItem("call_1", `{"temp_c":18}`),
		},
		// A trailing text turn has no tool_calls to exempt it from the replay rule.
		"text_only_trailing_turn": {
			deepSeekLiveUserItem("What is the weather in Paris?"),
			deepSeekLiveAssistantItem("It is 18C."),
			deepSeekLiveReasoningItem("Report the tool result."),
		},
		// Two turns: each must keep its own thinking.
		"two_turns_each_with_thinking": {
			deepSeekLiveUserItem("What is the weather in Paris?"),
			deepSeekLiveAssistantItem("Let me check."),
			deepSeekLiveReasoningItem("Call the weather tool."),
			deepSeekLiveFunctionCallItem("call_1", "Paris"),
			deepSeekLiveFunctionOutputItem("call_1", `{"temp_c":18}`),
			deepSeekLiveAssistantItem("It is 18C."),
			deepSeekLiveReasoningItem("Report the tool result."),
		},
		// A client that never saw call_id reuses one placeholder for every call.
		"parallel_calls_sharing_one_id": {
			deepSeekLiveUserItem("Weather in Paris and Lyon?"),
			deepSeekLiveAssistantItem("Checking both."),
			deepSeekLiveReasoningItem("Call the tool twice."),
			deepSeekLiveFunctionCallItem("undefined", "Paris"),
			deepSeekLiveFunctionCallItem("undefined", "Lyon"),
			deepSeekLiveFunctionOutputItem("undefined", `{"temp_c":18}`),
			deepSeekLiveFunctionOutputItem("undefined", `{"temp_c":22}`),
		},
		// Trimmed history: the call survived but its result did not.
		"call_without_output": {
			deepSeekLiveUserItem("What is the weather in Paris?"),
			deepSeekLiveAssistantItem("Checking."),
			deepSeekLiveReasoningItem("Call the weather tool."),
			deepSeekLiveFunctionCallItem("call_1", "Paris"),
		},
		// The result came back under an id that no longer matches the call.
		"output_with_mismatched_id": {
			deepSeekLiveUserItem("What is the weather in Paris?"),
			deepSeekLiveAssistantItem("Checking."),
			deepSeekLiveReasoningItem("Call the weather tool."),
			deepSeekLiveFunctionCallItem("call_1", "Paris"),
			deepSeekLiveFunctionOutputItem("call_other", `{"temp_c":18}`),
		},
		// A result whose call is gone entirely.
		"orphan_output": {
			deepSeekLiveUserItem("What is the weather in Paris?"),
			deepSeekLiveFunctionOutputItem("call_gone", `{"temp_c":18}`),
		},
	}

	for name, input := range scenarios {
		t.Run(name, func(t *testing.T) {
			chatReq, err := ConvertResponseAPIToChatCompletionRequest(&ResponseAPIRequest{
				Model: cfg.model,
				Input: input,
			})
			require.NoError(t, err)

			messages, _ := deepseekcompat.EnforceHistoryContract(chatReq.Messages)

			status, upstreamErr := postDeepSeekChatCompletion(t, cfg, messages)
			require.Equalf(t, http.StatusOK, status,
				"upstream rejected the converted history: %s", upstreamErr)
		})
	}
}

// TestDeepSeekLiveContractRejectsUnrepairedHistory pins why the repairs exist by
// sending the shapes they remove and asserting upstream still rejects them. A
// failure here means DeepSeek relaxed a rule, not that this gateway regressed.
// Parameters: t is the testing handle used for assertions.
// Returns: nothing; the test fails through t when a documented rule disappears.
func TestDeepSeekLiveContractRejectsUnrepairedHistory(t *testing.T) {
	cfg := requireDeepSeekLiveConfig(t)

	reasoning := "Call the weather tool."
	weatherCall := func(id, city string) model.Tool {
		return model.Tool{Id: id, Type: "function",
			Function: &model.Function{Name: "get_weather", Arguments: `{"city":"` + city + `"}`}}
	}

	cases := map[string]struct {
		messages    []model.Message
		wantMessage string
	}{
		"assistant_turn_without_replayed_reasoning": {
			messages: []model.Message{
				{Role: "user", Content: "What is the weather in Paris?"},
				{Role: "assistant", Content: "It is 18C."},
			},
			wantMessage: "The `reasoning_content` in the thinking mode must be passed back to the API.",
		},
		"tool_call_without_result": {
			messages: []model.Message{
				{Role: "user", Content: "What is the weather in Paris?"},
				{Role: "assistant", Content: "Checking.", ReasoningContent: &reasoning,
					ToolCalls: []model.Tool{weatherCall("call_1", "Paris")}},
			},
			wantMessage: "An assistant message with 'tool_calls' must be followed by tool messages",
		},
		"repeated_tool_call_id": {
			messages: []model.Message{
				{Role: "user", Content: "Weather in Paris and Lyon?"},
				{Role: "assistant", Content: "Checking.", ReasoningContent: &reasoning,
					ToolCalls: []model.Tool{weatherCall("undefined", "Paris"), weatherCall("undefined", "Lyon")}},
				{Role: "tool", ToolCallId: "undefined", Content: `{"temp_c":18}`},
				{Role: "tool", ToolCallId: "undefined", Content: `{"temp_c":22}`},
			},
			wantMessage: "Duplicate value for 'tool_call_id'",
		},
		"tool_result_without_call": {
			messages: []model.Message{
				{Role: "user", Content: "What is the weather in Paris?"},
				{Role: "tool", ToolCallId: "call_gone", Content: `{"temp_c":18}`},
			},
			wantMessage: "Messages with role 'tool' must be a response to a preceding message with 'tool_calls'",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			status, upstreamErr := postDeepSeekChatCompletion(t, cfg, tc.messages)
			require.Equal(t, http.StatusBadRequest, status,
				"upstream no longer rejects this shape; revisit the repairs it justifies")
			require.Contains(t, upstreamErr, tc.wantMessage)
		})
	}
}
