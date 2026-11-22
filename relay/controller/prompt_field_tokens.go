package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// chatPromptTokens estimates every provider-visible prompt field of a Chat
// Completions request: message content, role and name through the shared
// message counter, the remaining message fields such as assistant tool-call
// arguments and tool_call_id, and the request's tool, function and
// response_format schemas. Providers bill all of them as input, and tool
// schemas form part of the cacheable prompt prefix, so admission, tier
// selection and fallback usage must not omit them. Parameters: ctx carries the
// request logger, request is the prepared Chat payload (it is not modified) and
// model selects the tokenizer. It returns the estimated prompt token count.
func chatPromptTokens(ctx context.Context, request *relaymodel.GeneralOpenAIRequest, model string) int {
	if request == nil {
		return 0
	}
	tokens := openai.CountTokenMessages(ctx, request.Messages, model)
	for _, message := range request.Messages {
		// The shared counter already handled content, role and name.
		remaining := message
		remaining.Content = nil
		remaining.Role = ""
		remaining.Name = nil
		tokens += promptJSONTokens(remaining, model)
	}
	for _, value := range []any{request.Tools, request.Functions, request.ResponseFormat} {
		tokens += promptJSONTokens(value, model)
	}
	return tokens
}

// responseToolsPromptTokens counts Responses tool definitions once in their
// provider-visible form. ResponseAPITool.MarshalJSON emits a function tool both
// as flat name/description/parameters fields and, for compatibility, as a
// nested function object; the provider renders one definition, so the nested
// duplicate is excluded here and in namespace children. Parameters: tools is
// the request's tool list (it is not modified) and model selects the
// tokenizer. It returns the estimated tool definition tokens.
func responseToolsPromptTokens(tools []openai.ResponseAPITool, model string) int {
	if len(tools) == 0 {
		return 0
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return promptJSONTokens(tools, model)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return promptJSONTokens(tools, model)
	}
	return promptJSONTokens(withoutNestedFunctionDuplicates(decoded), model)
}

// withoutNestedFunctionDuplicates removes the nested function object from
// decoded Responses tools that already carry the flat definition. Parameter
// value is a decoded tool list or tool and is modified in place. It returns the
// same value for counting.
func withoutNestedFunctionDuplicates(value any) any {
	switch typed := value.(type) {
	case []any:
		for index := range typed {
			typed[index] = withoutNestedFunctionDuplicates(typed[index])
		}
	case map[string]any:
		if _, flat := typed["name"]; flat {
			if _, nested := typed["function"].(map[string]any); nested {
				delete(typed, "function")
			}
		}
		if children, ok := typed["tools"]; ok {
			typed["tools"] = withoutNestedFunctionDuplicates(children)
		}
	}
	return value
}

// promptJSONTokens counts one nonempty provider-visible JSON prompt field.
// Parameters: value is the field and model selects the tokenizer. It returns 0
// for an absent or empty field. Request fields come from JSON decoding, so a
// serialization failure is unreachable in practice; the printed value is then
// counted instead of dropping provider-visible input from the estimate.
func promptJSONTokens(value any, model string) int {
	encoded, err := json.Marshal(value)
	text := string(encoded)
	if err != nil {
		text = fmt.Sprint(value)
	}
	switch text {
	case "", "null", "[]", "{}":
		return 0
	}
	return openai.CountTokenText(text, model)
}
