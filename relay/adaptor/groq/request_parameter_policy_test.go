package groq

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestGroqChatParameterPolicy verifies final-wire compatibility without SDKs.
func TestGroqChatParameterPolicy(t *testing.T) {
	t.Parallel()
	body := []byte(`{"model":"openai/gpt-oss-120b","messages":[{"role":"assistant","name":"agent","content":"hello","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}],"provider":{"id":9007199254740993}},{"role":"tool","name":"lookup","tool_call_id":"call_1","content":"ok"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"logprobs":false,"top_logprobs":0,"logit_bias":{},"top_k":0,"temperature":0,"top_p":0.9,"n":2,"max_completion_tokens":42,"metadata":{"id":9007199254740993}}`)
	original := bytes.Clone(body)
	wire, removed := normalizeGroqChatParameters(body)
	if removed != 6 {
		t.Fatalf("removed=%d, want 6", removed)
	}
	if !bytes.Equal(body, original) {
		t.Fatal("modified shared input")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(wire, &root); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"logprobs", "top_logprobs", "logit_bias", "top_k"} {
		if _, ok := root[key]; ok {
			t.Errorf("unsupported field %s leaked", key)
		}
	}
	for key, want := range map[string]string{"temperature": "0", "top_p": "0.9", "n": "2", "max_completion_tokens": "42", "metadata": `{"id":9007199254740993}`} {
		if string(root[key]) != want {
			t.Errorf("%s=%s, want %s", key, root[key], want)
		}
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(root["messages"], &messages); err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if _, ok := message["name"]; ok {
			t.Fatal("message name leaked")
		}
	}
	if string(messages[1]["tool_call_id"]) != `"call_1"` || !bytes.Contains(messages[0]["tool_calls"], []byte(`"name":"lookup"`)) || !bytes.Contains(root["tools"], []byte(`"name":"lookup"`)) {
		t.Fatal("tool identity was damaged")
	}
	if string(messages[0]["provider"]) != `{"id":9007199254740993}` {
		t.Fatal("nested extension precision lost")
	}
	again, count := normalizeGroqChatParameters(wire)
	if count != 0 || !bytes.Equal(wire, again) {
		t.Fatal("cleanup is not idempotent")
	}
}

// TestGroqChatParameterPolicyBoundaries keeps non-chat and malformed data intact.
func TestGroqChatParameterPolicyBoundaries(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{`, `null`, `[]`, `{"input":"hi","temperature":0}`, `{"messages":"invalid","logprobs":true}`, `{"messages":[null,42,{},"opaque"]}`, `{"messages":[],"temperature":0}`, `{"messages":null}`} {
		t.Run(raw, func(t *testing.T) {
			wire, count := normalizeGroqChatParameters([]byte(raw))
			if count != 0 || string(wire) != raw {
				t.Fatalf("unexpected modification: %d %s", count, wire)
			}
		})
	}
}
