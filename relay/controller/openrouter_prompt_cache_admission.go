package controller

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// inspectOpenRouterPromptCacheWrites reports whether the exact prepared payload
// permits default 30m OpenAI cache writes through OpenRouter. Parameter payload
// is a parsed request or raw provider JSON. It returns the write intent or an
// error, rejecting 12h writes because OpenRouter combines both lifetimes in one
// usage counter. The bounded native scanner also inspects Chat messages through
// a read-only input projection; provider bytes and native behavior are unchanged.
// Source checked 2026-11-22:
// https://openrouter.ai/docs/guides/best-practices/prompt-caching
func inspectOpenRouterPromptCacheWrites(payload any) (bool, error) {
	raw, err := tierAdmissionPayloadJSON(payload)
	if err != nil {
		return false, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return false, errors.Wrap(err, "decode OpenRouter cache request")
	}
	intent, err := openai.InspectPromptCacheWriteIntentRoot(root)
	if err != nil {
		return false, err
	}
	if messages, ok := root["messages"]; ok {
		chat, err := openai.InspectPromptCacheWriteIntentRoot(map[string]json.RawMessage{"input": messages})
		if err != nil {
			return false, err
		}
		intent.Breakpoints += chat.Breakpoints
	}
	if input, ok := root["input"]; ok {
		// The structural Anthropic scanner normally reads Chat messages.
		// OpenRouter also translates these markers inside Responses input,
		// dropping their TTL and writing through the OpenAI 30m bucket.
		write5m, write1h, err := tierAdmissionCacheWrites(map[string]json.RawMessage{"messages": input})
		if err != nil {
			return false, err
		}
		if write5m || write1h {
			return true, nil
		}
	}
	return intent.Writes(), nil
}
