package groq

import "encoding/json"

// normalizeGroqChatParameters removes fields explicitly rejected by Groq Chat
// Completions. It preserves opaque JSON, tool identities, supported sampling,
// output limits and invalid values that need an ordinary validation error.
// Input bytes are never modified. Malformed input is left for normal validation.
// Source (verified 2026-09-24): https://console.groq.com/docs/openai
func normalizeGroqChatParameters(body []byte) ([]byte, int) {
	var root map[string]json.RawMessage
	if json.Unmarshal(body, &root) != nil || root == nil {
		return body, 0
	}
	// Audio, embedding and native Responses requests do not use this contract.
	var messages []json.RawMessage
	if json.Unmarshal(root["messages"], &messages) != nil {
		return body, 0
	}
	removed := 0
	for _, key := range []string{"logprobs", "logit_bias", "top_logprobs", "top_k"} {
		if _, ok := root[key]; ok {
			delete(root, key)
			removed++
		}
	}
	messagesChanged := false
	for i, raw := range messages {
		var message map[string]json.RawMessage
		if json.Unmarshal(raw, &message) != nil || message == nil {
			continue
		}
		if _, ok := message["name"]; !ok {
			continue
		}
		delete(message, "name")
		encoded, err := json.Marshal(message)
		if err != nil {
			return body, 0
		}
		messages[i] = encoded
		messagesChanged = true
		removed++
	}
	if messagesChanged {
		encoded, err := json.Marshal(messages)
		if err != nil {
			return body, 0
		}
		root["messages"] = encoded
	}
	if removed == 0 {
		return body, 0
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return body, 0
	}
	return encoded, removed
}
