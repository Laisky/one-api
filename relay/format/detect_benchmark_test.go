package format

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

var benchmarkDetectFormatPlainText = []byte(`{
	"model":"gpt-4.1",
	"messages":[
		{"role":"system","content":"You are helpful"},
		{"role":"user","content":"Explain Kubernetes operators"},
		{"role":"assistant","content":"Sure."},
		{"role":"user","content":"Continue with examples"}
	]
}`)

var benchmarkPlainTextMessages = json.RawMessage(`[
	{"role":"system","content":"You are helpful"},
	{"role":"user","content":"Explain Kubernetes operators"},
	{"role":"assistant","content":"Sure."},
	{"role":"user","content":"Continue with examples"}
]`)

var benchmarkOpenAITools = json.RawMessage(`[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"payload":{"type":"string","description":"` + strings.Repeat("A", 4096) + `"}}}}}]`)

var benchmarkDetectFormatOpenAITool = []byte(`{
	"model":"gpt-4.1",
	"messages":[{"role":"user","content":"look this up"}],
	"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"payload":{"type":"string","description":"` + strings.Repeat("A", 4096) + `"}}}}}]
}`)

type legacyRequestProbe struct {
	Model              string          `json:"model,omitempty"`
	Messages           json.RawMessage `json:"messages,omitempty"`
	Input              json.RawMessage `json:"input,omitempty"`
	Instructions       *string         `json:"instructions,omitempty"`
	PreviousResponseId *string         `json:"previous_response_id,omitempty"`
	Conversation       json.RawMessage `json:"conversation,omitempty"`
	Prompt             json.RawMessage `json:"prompt,omitempty"`
	System             any             `json:"system,omitempty"`
	MaxOutputTokens    *int            `json:"max_output_tokens,omitempty"`
	Tools              json.RawMessage `json:"tools,omitempty"`
}

type legacyMessageProbe struct {
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
}

// hasClaudeContentBlocksLegacy preserves the pre-optimization content-block scan for differential tests and benchmarks.
func hasClaudeContentBlocksLegacy(messagesRaw json.RawMessage) bool {
	var messages []legacyMessageProbe
	if err := json.Unmarshal(messagesRaw, &messages); err != nil {
		return false
	}

	for _, msg := range messages {
		var contentArray []contentBlockProbe
		if err := json.Unmarshal(msg.Content, &contentArray); err == nil {
			for _, block := range contentArray {
				switch block.Type {
				case "tool_use", "tool_result", "thinking":
					return true
				}
			}
		}
	}

	return false
}

// isClaudeToolFormatLegacy preserves the pre-optimization nested tool-schema decoder for differential tests and benchmarks.
func isClaudeToolFormatLegacy(toolsRaw json.RawMessage) bool {
	var tools []toolProbe
	if err := json.Unmarshal(toolsRaw, &tools); err != nil {
		return false
	}

	for _, tool := range tools {
		if len(tool.InputSchema) > 0 && tool.Name != "" {
			return true
		}

		if tool.Type == "function" && len(tool.Function) > 0 {
			var fnProbe struct {
				Parameters  json.RawMessage `json:"parameters,omitempty"`
				InputSchema json.RawMessage `json:"input_schema,omitempty"`
			}
			if err := json.Unmarshal(tool.Function, &fnProbe); err == nil && len(fnProbe.InputSchema) > 0 {
				return true
			}
		}
	}

	return false
}

// detectFormatLegacy preserves the pre-optimization detector for end-to-end differential tests and benchmarks.
func detectFormatLegacy(body []byte) (APIFormat, error) {
	if len(body) == 0 {
		return Unknown, errors.New("empty request body")
	}

	var probe legacyRequestProbe
	if err := json.Unmarshal(body, &probe); err != nil {
		return Unknown, errors.Wrap(err, "failed to parse request body for format detection")
	}

	if len(probe.Input) > 0 && len(probe.Messages) == 0 {
		return ResponseAPI, nil
	}
	if probe.MaxOutputTokens != nil && len(probe.Messages) == 0 {
		return ResponseAPI, nil
	}
	if probe.Instructions != nil && len(probe.Messages) == 0 {
		return ResponseAPI, nil
	}
	if len(probe.Messages) == 0 {
		if probe.PreviousResponseId != nil {
			return ResponseAPI, nil
		}
		if isNonEmptyJSONValue(probe.Conversation) {
			return ResponseAPI, nil
		}
		if isResponseAPIPromptObject(probe.Prompt) {
			return ResponseAPI, nil
		}
		return Unknown, nil
	}

	if hasClaudeContentBlocksLegacy(probe.Messages) {
		return ClaudeMessages, nil
	}
	if len(probe.Tools) > 0 && isClaudeToolFormatLegacy(probe.Tools) {
		return ClaudeMessages, nil
	}

	return Unknown, nil
}

// TestDetectFormatBehaviorEquivalent compares externally observable detector results against the exact legacy path.
func TestDetectFormatBehaviorEquivalent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body []byte
	}{
		{name: "plain text", body: benchmarkDetectFormatPlainText},
		{name: "openai tool schema", body: benchmarkDetectFormatOpenAITool},
		{name: "claude tool use", body: []byte(`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"1"}]}]}`)},
		{name: "wrong-type role", body: []byte(`{"messages":[{"role":1,"content":[{"type":"tool_use","id":"1"}]}]}`)},
		{name: "wrong-type model", body: []byte(`{"model":123,"messages":[{"role":"user","content":"hello"}]}`)},
		{name: "responses input", body: []byte(`{"input":"hello"}`)},
		{name: "invalid json", body: []byte(`{"messages":`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, wantErr := detectFormatLegacy(tc.body)
			got, gotErr := DetectFormat(tc.body)
			require.Equal(t, want, got)
			if wantErr == nil {
				require.NoError(t, gotErr)
			} else {
				require.EqualError(t, gotErr, wantErr.Error())
			}
		})
	}
}

// TestHasClaudeContentBlocksBehaviorEquivalent compares the optimized content-block scan against the exact legacy algorithm.
func TestHasClaudeContentBlocksBehaviorEquivalent(t *testing.T) {
	t.Parallel()
	cases := []json.RawMessage{
		benchmarkPlainTextMessages,
		json.RawMessage(`[{"role":"assistant","content":null}]`),
		json.RawMessage(`[{"role":"user","content":{"text":"hello"}}]`),
		json.RawMessage(`[{"role":"user","content":[{"type":"text","text":"hello"}]}]`),
		json.RawMessage(`[{"role":"assistant","content":[{"type":"tool_use","id":"1"}]}]`),
		json.RawMessage(`[{"role":"user","content":[{"type":"tool_result","tool_use_id":"1"}]}]`),
		json.RawMessage(`[{"role":"assistant","content":[{"type":"thinking","thinking":"..."}]}]`),
		json.RawMessage(`[{"role":1,"content":[{"type":"tool_use","id":"1"}]}]`),
	}

	for _, raw := range cases {
		require.Equal(t, hasClaudeContentBlocksLegacy(raw), hasClaudeContentBlocks(raw), string(raw))
	}
}

// TestIsClaudeToolFormatBehaviorEquivalent compares optimized tool detection against the exact legacy algorithm.
func TestIsClaudeToolFormatBehaviorEquivalent(t *testing.T) {
	t.Parallel()
	cases := []json.RawMessage{
		benchmarkOpenAITools,
		json.RawMessage(`[{"name":"lookup","input_schema":{"type":"object"}}]`),
		json.RawMessage(`[{"type":"function","function":{"name":"lookup","input_schema":{"type":"object"}}}]`),
		json.RawMessage(`[{"type":"function","function":{"name":"lookup","parameters":{}}}]`),
		json.RawMessage(`not-json`),
	}

	for _, raw := range cases {
		require.Equal(t, isClaudeToolFormatLegacy(raw), isClaudeToolFormat(raw), string(raw))
	}
}

// BenchmarkClaudeContentBlockScanPlainText measures the plain-text message scan before and after skipping impossible array decodes.
func BenchmarkClaudeContentBlockScanPlainText(b *testing.B) {
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = hasClaudeContentBlocksLegacy(benchmarkPlainTextMessages)
		}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = hasClaudeContentBlocks(benchmarkPlainTextMessages)
		}
	})
}

// BenchmarkClaudeToolFormatOpenAISchema measures tool-format detection with a large OpenAI parameters schema.
func BenchmarkClaudeToolFormatOpenAISchema(b *testing.B) {
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = isClaudeToolFormatLegacy(benchmarkOpenAITools)
		}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = isClaudeToolFormat(benchmarkOpenAITools)
		}
	})
}

// BenchmarkDetectFormatPlainText measures end-to-end detection for a typical multi-message text request.
func BenchmarkDetectFormatPlainText(b *testing.B) {
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := detectFormatLegacy(benchmarkDetectFormatPlainText); err != nil {
				b.Fatal(err)
			}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := DetectFormat(benchmarkDetectFormatPlainText); err != nil {
				b.Fatal(err)
			}
	})
}

// BenchmarkDetectFormatOpenAIToolSchema measures end-to-end detection for a request with a large OpenAI tool schema.
func BenchmarkDetectFormatOpenAIToolSchema(b *testing.B) {
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := detectFormatLegacy(benchmarkDetectFormatOpenAITool); err != nil {
				b.Fatal(err)
			}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := DetectFormat(benchmarkDetectFormatOpenAITool); err != nil {
				b.Fatal(err)
			}
	})
}
