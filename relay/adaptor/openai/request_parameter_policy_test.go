package openai

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// TestReasoningWireSamplingPolicy exercises wire behavior without provider SDKs.
func TestReasoningWireSamplingPolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, request, defaultEffort string
		supportsNone, blocked        bool
	}{
		{"luna implicit medium", `{}`, "medium", true, true},
		{"luna empty reasoning", `{"reasoning":{}}`, "medium", true, true},
		{"luna summary only", `{"reasoning":{"summary":"auto"}}`, "medium", true, true},
		{"luna explicit none", `{"reasoning":{"effort":"none"}}`, "medium", true, false},
		{"luna explicit high", `{"reasoning":{"effort":"high"}}`, "medium", true, true},
		{"astra none unsupported", `{"reasoning":{"effort":"none"}}`, "medium", false, true},
		{"gpt5 minimal still reasons", `{"reasoning":{"effort":"minimal"}}`, "medium", false, true},
		{"gpt52 default none", `{}`, "none", true, false},
		{"gpt52 high", `{"reasoning":{"effort":"high"}}`, "none", true, true},
		{"chat none", `{"messages":[],"reasoning_effort":"none"}`, "medium", true, false},
		{"chat high", `{"messages":[],"reasoning_effort":"high"}`, "none", true, true},
		{"responses effort wins", `{"input":"hello","reasoning":{"effort":"high"},"reasoning_effort":"none"}`, "medium", true, true},
		{"chat effort wins", `{"messages":[],"reasoning":{"effort":"none"},"reasoning_effort":"high"}`, "medium", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var root map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.request), &root); err != nil {
				t.Fatal(err)
			}
			for key, value := range map[string]string{"temperature": "0", "top_p": "0.9", "logprobs": "false", "top_logprobs": "0", "include": `["reasoning.encrypted_content","message.output_text.logprobs","file_search_call.results"]`, "metadata": `{"temperature":0,"id":9007199254740993}`, "max_output_tokens": "128000", "previous_response_id": `"resp_kept"`} {
				root[key] = json.RawMessage(value)
			}
			metadata := bytes.Clone(root["metadata"])
			removed := normalizeReasoningParameterMap(root, tc.defaultEffort, tc.supportsNone)
			for _, key := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
				_, present := root[key]
				if present == tc.blocked {
					t.Errorf("%s present=%v; blocked=%v", key, present, tc.blocked)
				}
			}
			if tc.blocked {
				if len(removed) != 5 {
					t.Errorf("removed=%v, want 5 diagnostics", removed)
				}
				var include []string
				if err := json.Unmarshal(root["include"], &include); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(include, []string{"reasoning.encrypted_content", "file_search_call.results"}) {
					t.Errorf("include=%v", include)
				}
			} else if len(removed) != 0 {
				t.Errorf("supported fields removed: %v", removed)
			}
			if !bytes.Equal(metadata, root["metadata"]) {
				t.Fatal("nested content modified")
			}
			if string(root["max_output_tokens"]) != "128000" || string(root["previous_response_id"]) != `"resp_kept"` {
				t.Fatal("semantic controls removed")
			}
			before, _ := json.Marshal(root)
			if got := normalizeReasoningParameterMap(root, tc.defaultEffort, tc.supportsNone); len(got) != 0 {
				t.Errorf("not idempotent: %v", got)
			}
			after, _ := json.Marshal(root)
			if !bytes.Equal(before, after) {
				t.Fatal("second pass changed payload")
			}
		})
	}
}

// TestReasoningIncludeFiltering preserves raw unknown items and never mutates the input bytes.
func TestReasoningIncludeFiltering(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`["message.output_text.logprobs"]`, `["message.output_text.logprobs","message.output_text.logprobs"]`, `[9007199254740993,"message.output_text.logprobs"]`, `null`, `{"provider":"value"}`} {
		t.Run(raw, func(t *testing.T) {
			input := []byte(raw)
			original := bytes.Clone(input)
			root := map[string]json.RawMessage{"include": input}
			normalizeReasoningParameterMap(root, "medium", false)
			if !bytes.Equal(input, original) {
				t.Fatal("mutated shared backing bytes")
			}
			if raw == `[9007199254740993,"message.output_text.logprobs"]` && string(root["include"]) != `[9007199254740993]` {
				t.Fatal(string(root["include"]))
			}
			if raw == `["message.output_text.logprobs"]` || raw == `["message.output_text.logprobs","message.output_text.logprobs"]` {
				if _, ok := root["include"]; ok {
					t.Fatal("empty include must be omitted")
				}
			}
			if raw == `null` || raw == `{"provider":"value"}` {
				if !bytes.Equal(root["include"], original) {
					t.Fatal("unknown include modified")
				}
			}
		})
	}
}
