package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestParseModels(t *testing.T) {
	cases := map[string][]string{
		"gpt-4":                     {"gpt-4"},
		"gpt-4,claude-3":            {"gpt-4", "claude-3"},
		"gpt-4; claude-3 \n gemini": {"gpt-4", "claude-3", "gemini"},
		"  gpt-4  ,  claude-3   ":   {"gpt-4", "claude-3"},
		"gpt-4\n\nclaude-3":         {"gpt-4", "claude-3"},
		"gpt-4 claude-3":            {"gpt-4", "claude-3"},
	}

	for input, want := range cases {
		got, err := parseModels(input)
		if err != nil {
			t.Fatalf("parseModels(%q) returned error: %v", input, err)
		}
		if len(got) != len(want) {
			t.Fatalf("parseModels(%q) length = %d, want %d", input, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("parseModels(%q)[%d] = %q, want %q", input, i, got[i], want[i])
			}
		}
	}
}

func TestParseModelsEmpty(t *testing.T) {
	got, err := parseModels("   ")
	if err != nil {
		t.Fatalf("parseModels empty error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("parseModels empty length = %d, want 0", len(got))
	}
}

func TestEvaluateResponseChatCompletionSuccess(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`)
	spec := requestSpec{Type: requestTypeChatCompletion, Expectation: expectationDefault}
	success, reason := evaluateResponse(spec, body)
	if !success {
		t.Fatalf("expected success, got failure: %s", reason)
	}
}

func TestEvaluateResponseIgnoresEmptyErrorObject(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}],"error":{"message":"","type":"","param":"","code":null}}`)
	spec := requestSpec{Type: requestTypeChatCompletion, Expectation: expectationDefault}
	success, reason := evaluateResponse(spec, body)
	if !success {
		t.Fatalf("expected success despite empty error object, got: %s", reason)
	}
}

func TestEvaluateResponseResponseAPIChoicesFallback(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"role":"assistant","content":"hi"}}],"object":"chat.completion"}`)
	spec := requestSpec{Type: requestTypeResponseAPI, Expectation: expectationDefault}
	success, reason := evaluateResponse(spec, body)
	if !success {
		t.Fatalf("expected Response API fallback success, got: %s", reason)
	}
}

func TestEvaluateResponseChatToolInvocation(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"tool_calls":[{"id":"tool_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"San Francisco\"}"}}]}}]}`)
	spec := requestSpec{Type: requestTypeChatCompletion, Expectation: expectationToolInvocation}
	success, reason := evaluateResponse(spec, body)
	if !success {
		t.Fatalf("expected tool invocation success, got: %s", reason)
	}
}

func TestEvaluateResponseResponseAPIToolInvocation(t *testing.T) {
	body := []byte(`{"required_action":{"type":"submit_tool_outputs","submit_tool_outputs":{"tool_calls":[{"id":"call_1","name":"get_weather","arguments":"{\"location\":\"San Francisco\"}"}]}}}`)
	spec := requestSpec{Type: requestTypeResponseAPI, Expectation: expectationToolInvocation}
	success, reason := evaluateResponse(spec, body)
	if !success {
		t.Fatalf("expected Response API tool invocation success, got: %s", reason)
	}
}

func TestEvaluateResponseClaudeToolInvocation(t *testing.T) {
	body := []byte(`{"content":[{"type":"tool_use","name":"get_weather","input":{"location":"San Francisco"}}]}`)
	spec := requestSpec{Type: requestTypeClaudeMessages, Expectation: expectationToolInvocation}
	success, reason := evaluateResponse(spec, body)
	if !success {
		t.Fatalf("expected Claude tool invocation success, got: %s", reason)
	}
}

func TestEvaluateResponseClaudeToolInvocationChoices(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"tool_calls":[{"type":"function","function":{"name":"get_weather"}}]}}]}`)
	spec := requestSpec{Type: requestTypeClaudeMessages, Expectation: expectationToolInvocation}
	success, reason := evaluateResponse(spec, body)
	if !success {
		t.Fatalf("expected Claude choices tool invocation success, got: %s", reason)
	}
}

func TestIsUnsupportedCombinationResponse(t *testing.T) {
	body := []byte("{\"error\":{\"message\":\"unknown field `messages`\"}}")
	if !isUnsupportedCombination(requestTypeResponseAPI, false, http.StatusBadRequest, body, "") {
		t.Fatalf("expected combination to be marked unsupported")
	}
}

func TestEvaluateStreamResponseSuccess(t *testing.T) {
	data := []byte("data: {\"id\":\"resp_123\",\"error\":null}\n\n")
	spec := requestSpec{Type: requestTypeResponseAPI, Expectation: expectationDefault}
	success, reason := evaluateStreamResponse(spec, data)
	if !success {
		t.Fatalf("expected stream success, got failure: %s", reason)
	}
}

func TestEvaluateStreamResponseToolInvocationChat(t *testing.T) {
	data := []byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_1\"}]}}]}\n\n")
	spec := requestSpec{Type: requestTypeChatCompletion, Expectation: expectationToolInvocation}
	success, reason := evaluateStreamResponse(spec, data)
	if !success {
		t.Fatalf("expected stream tool invocation success, got failure: %s", reason)
	}
}

func TestEvaluateStreamResponseToolInvocationResponseAPIItem(t *testing.T) {
	data := []byte("data: {\"type\":\"response.output_item.added\",\"item\":{\"type\":\"function_call\",\"name\":\"get_weather\"}}\n\n")
	spec := requestSpec{Type: requestTypeResponseAPI, Expectation: expectationToolInvocation}
	success, reason := evaluateStreamResponse(spec, data)
	if !success {
		t.Fatalf("expected Response API item tool invocation success, got failure: %s", reason)
	}
}

func TestEvaluateStreamResponseToolInvocationMissing(t *testing.T) {
	data := []byte("data: {\"choices\":[{\"delta\":{}}]}\n\n")
	spec := requestSpec{Type: requestTypeChatCompletion, Expectation: expectationToolInvocation}
	success, reason := evaluateStreamResponse(spec, data)
	if success {
		t.Fatalf("expected stream tool invocation failure, got success")
	}
	if reason == "" {
		t.Fatalf("expected failure reason when tool invocation is absent")
	}
}

func TestIsUnsupportedCombinationStream(t *testing.T) {
	body := []byte("streaming is not supported")
	if !isUnsupportedCombination(requestTypeChatCompletion, true, http.StatusBadRequest, body, "") {
		t.Fatalf("expected streaming combination to be marked unsupported")
	}
}

func TestIsUnsupportedCombinationResponseFormatUnavailable(t *testing.T) {
	body := []byte("{\"error\":{\"message\":\"This response_format type is unavailable now\"}}")
	if isUnsupportedCombination(requestTypeChatCompletion, false, http.StatusBadRequest, body, "") {
		t.Fatalf("response_format unavailable should be treated as failure now")
	}
}

func TestEvaluateStreamResponseStructuredSplitTokens(t *testing.T) {
	partials := []string{
		`{"topic"`,
		`": "AI adoption"`,
		`", "conf"`,
		`idence":0.95"`,
	}
	var buf bytes.Buffer
	for _, fragment := range partials {
		chunk := map[string]any{
			"type": "content_block_delta",
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": fragment,
			},
		}
		payload, err := json.Marshal(chunk)
		if err != nil {
			t.Fatalf("failed to marshal chunk: %v", err)
		}
		buf.WriteString("data: ")
		buf.Write(payload)
		buf.WriteByte('\n')
	}
	buf.WriteString("data: [DONE]\n")
	data := buf.Bytes()
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	fragments := &strings.Builder{}
	for _, raw := range lines {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[len("data:"):])
		if bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(payload, &obj); err != nil {
			t.Fatalf("failed to unmarshal payload %q: %v", payload, err)
		}
		appendStructuredFragments(obj, fragments)
	}
	if !structuredOutputSatisfiedBytes([]byte(fragments.String())) {
		t.Fatalf("expected fragments to satisfy structured detection, fragments=%q", fragments.String())
	}
	spec := requestSpec{Type: requestTypeClaudeMessages, Expectation: expectationStructuredOutput}
	success, reason := evaluateStreamResponse(spec, data)
	if !success {
		t.Fatalf("expected structured stream success despite split tokens, got: %s", reason)
	}
}
