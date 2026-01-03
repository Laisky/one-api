package aws

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestConvertConverseResponseToQwenUsageMapping(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	in := int32(11)
	out := int32(22)
	total := int32(33)

	converse := &bedrockruntime.ConverseOutput{
		Output: &types.ConverseOutputMemberMessage{Value: types.Message{Role: types.ConversationRole("assistant")}},
		Usage: &types.TokenUsage{
			InputTokens:  aws.Int32(in),
			OutputTokens: aws.Int32(out),
			TotalTokens:  aws.Int32(total),
		},
	}

	resp := convertConverseResponseToQwen(c, converse, "qwen.qwen3-coder-480b-a35b-v1:0")

	b, err := json.Marshal(resp)
	require.NoError(t, err, "marshal qwen response")

	js := string(b)
	require.True(t, strings.Contains(js, "\"prompt_tokens\":11"), "expected prompt_tokens:11 in json, got %s", js)
	require.True(t, strings.Contains(js, "\"completion_tokens\":22"), "expected completion_tokens:22 in json, got %s", js)
	require.True(t, strings.Contains(js, "\"total_tokens\":33"), "expected total_tokens:33 in json, got %s", js)
}

func TestConvertRequestMapsToolsAndReasoning(t *testing.T) {
	t.Parallel()
	temp := 0.5
	topP := 0.9
	reasoning := "high"
	stop := []any{"done", "halt"}

	req := relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "system", Content: "you are helpful"},
			{Role: "user", Content: "hi"},
		},
		Model:           "qwen3-32b",
		MaxTokens:       4096,
		Temperature:     &temp,
		TopP:            &topP,
		Stop:            stop,
		ReasoningEffort: &reasoning,
		Tools: []relaymodel.Tool{
			{
				Type: "function",
				Function: &relaymodel.Function{
					Name:        "calculate",
					Description: "perform calculation",
					Parameters: map[string]any{
						"type": "object",
					},
				},
			},
		},
		ToolChoice: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "calculate",
			},
		},
	}

	converted := ConvertRequest(req)
	require.NotNil(t, converted, "expected non-nil converted request")
	require.Equal(t, 4096, converted.MaxTokens, "unexpected max tokens")
	require.NotNil(t, converted.Temperature, "temperature should not be nil")
	require.Equal(t, temp, *converted.Temperature, "temperature not preserved")
	require.NotNil(t, converted.TopP, "top_p should not be nil")
	require.Equal(t, topP, *converted.TopP, "top_p not preserved")
	require.True(t, reflect.DeepEqual(converted.Stop, []string{"done", "halt"}), "unexpected stop sequences: %+v", converted.Stop)
	require.NotNil(t, converted.ReasoningEffort, "reasoning effort should not be nil")
	require.Equal(t, reasoning, *converted.ReasoningEffort, "reasoning effort not preserved")
	require.Len(t, converted.Tools, 1, "expected 1 tool")
	require.Equal(t, "calculate", converted.Tools[0].Function.Name, "unexpected tool name")

	toolChoice, ok := converted.ToolChoice.(map[string]any)
	require.True(t, ok, "expected tool choice map, got %T", converted.ToolChoice)
	require.Equal(t, "function", toolChoice["type"], "unexpected tool choice type")
	fn, ok := toolChoice["function"].(map[string]any)
	require.True(t, ok, "expected function map in tool choice")
	require.Equal(t, "calculate", fn["name"], "unexpected tool choice function name")
}

func TestConvertMessagesMarshalsNonStringArguments(t *testing.T) {
	t.Parallel()
	args := map[string]any{"foo": "bar"}
	messages := []relaymodel.Message{
		{
			Role: "assistant",
			ToolCalls: []relaymodel.Tool{
				{
					Id:   "call-1",
					Type: "function",
					Function: &relaymodel.Function{
						Name:      "do",
						Arguments: args,
					},
				},
			},
		},
	}

	converted := ConvertMessages(messages)
	require.Len(t, converted, 1, "expected 1 message")
	require.Len(t, converted[0].ToolCalls, 1, "expected 1 tool call")

	var decoded map[string]any
	err := json.Unmarshal([]byte(converted[0].ToolCalls[0].Function.Arguments), &decoded)
	require.NoError(t, err, "arguments should be json")
	require.Equal(t, "bar", decoded["foo"], "unexpected tool arguments")
}

func TestConvertQwenToConverseRequestIncludesReasoningConfig(t *testing.T) {
	t.Parallel()
	reasoning := "medium"
	req := &Request{
		Messages: []Message{
			{Role: "system", Content: "guide"},
			{Role: "user", Content: "hello"},
		},
		ReasoningEffort: &reasoning,
		Tools: []QwenTool{
			{
				Type: "function",
				Function: QwenToolSpec{
					Name:        "calculate",
					Description: "math helper",
					Parameters: map[string]any{
						"type": "object",
					},
				},
			},
		},
		ToolChoice: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "calculate",
			},
		},
	}

	converseReq, err := convertQwenToConverseRequest(req, "qwen3-test")
	require.NoError(t, err, "convert request")
	require.NotNil(t, converseReq.ToolConfig, "expected tool config to be set")
	require.Len(t, converseReq.ToolConfig.Tools, 1, "expected one tool specification")

	toolSpec, ok := converseReq.ToolConfig.Tools[0].(*types.ToolMemberToolSpec)
	require.True(t, ok, "unexpected tool type: %T", converseReq.ToolConfig.Tools[0])
	require.NotNil(t, toolSpec.Value.Name, "tool name should not be nil")
	require.Equal(t, "calculate", *toolSpec.Value.Name, "unexpected tool name")
	require.NotNil(t, converseReq.AdditionalModelRequestFields, "expected reasoning config to be included")

	b, err := json.Marshal(converseReq.AdditionalModelRequestFields)
	require.NoError(t, err, "marshal additional fields")
	require.NotEmpty(t, b, "expected additional fields to marshal to json, got empty payload")
}

func TestConvertQwenToConverseRequestInvalidToolArguments(t *testing.T) {
	t.Parallel()
	req := &Request{
		Messages: []Message{
			{
				Role: "assistant",
				ToolCalls: []QwenToolCall{
					{
						ID:   "call-1",
						Type: "function",
						Function: QwenToolFunction{
							Name:      "calc",
							Arguments: "{not-json",
						},
					},
				},
			},
		},
	}

	_, err := convertQwenToConverseRequest(req, "qwen3-test")
	require.Error(t, err, "expected error converting invalid tool arguments")
}
