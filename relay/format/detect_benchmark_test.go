package format

import (
	"encoding/json"
	"strings"
	"testing"

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

var benchmarkDetectFormatLargeSystem = []byte(`{
	"model":"claude-sonnet-4",
	"system":[{"type":"text","text":"` + strings.Repeat("A", 4096) + `"}],
	"messages":[
		{"role":"user","content":"Explain Kubernetes operators"},
		{"role":"assistant","content":"Sure."},
		{"role":"user","content":"Continue with examples"}
	]
}`)

var benchmarkOpenAITools = json.RawMessage(`[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"payload":{"type":"string","description":"` + strings.Repeat("A", 4096) + `"}}}}}]`)

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
			if err := json.Unmarshal(tool.Function, &fnProbe); err == nil {
				if len(fnProbe.InputSchema) > 0 {
					return true
				}
			}
		}
	}

	return false
}

// TestDetectFormatScalarContentBehavior verifies scalar and structured message cases retain their expected API-format decisions.
func TestDetectFormatScalarContentBehavior(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want APIFormat
	}{
		{name: "string content", body: `{"messages":[{"role":"user","content":"hello"}]}`, want: Unknown},
		{name: "null content", body: `{"messages":[{"role":"assistant","content":null}]}`, want: Unknown},
		{name: "object content", body: `{"messages":[{"role":"user","content":{"text":"hello"}}]}`, want: Unknown},
		{name: "claude array content", body: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"1"}]}]}`, want: ClaudeMessages},
		{name: "large unused fields stay ambiguous", body: string(benchmarkDetectFormatLargeSystem), want: Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectFormat([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
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

// BenchmarkRequestProbeDecodeLargeUnusedFields measures decoding when large top-level fields are irrelevant to format detection.
func BenchmarkRequestProbeDecodeLargeUnusedFields(b *testing.B) {
	b.Run("legacy", func(b *testing.B) {
		b.ReportAllocs()
		var probe legacyRequestProbe
		for b.Loop() {
			probe = legacyRequestProbe{}
			if err := json.Unmarshal(benchmarkDetectFormatLargeSystem, &probe); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("optimized", func(b *testing.B) {
		b.ReportAllocs()
		var probe requestProbe
		for b.Loop() {
			probe = requestProbe{}
			if err := json.Unmarshal(benchmarkDetectFormatLargeSystem, &probe); err != nil {
				b.Fatal(err)
			}
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

// BenchmarkDetectFormatPlainText records the optimized end-to-end detector cost for a typical multi-message text request.
func BenchmarkDetectFormatPlainText(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := DetectFormat(benchmarkDetectFormatPlainText); err != nil {
			b.Fatal(err)
		}
	}
}
