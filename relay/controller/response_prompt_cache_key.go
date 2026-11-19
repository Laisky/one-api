package controller

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// promptCacheKeyRootKey is OpenAI's root prompt_cache_key request field.
const promptCacheKeyRootKey = "prompt_cache_key"

// syncResponsePromptCacheKey makes the native Responses body carry exactly the
// tenant-scoped prompt_cache_key from the typed request. The client's raw
// value is never forwarded: it is replaced by key.Forwarded() when
// openai.ScopePromptCacheKey scoped the key for a native OpenAI destination,
// and removed otherwise. Parameters: root is the decoded body (modified in
// place) and key is the typed request's key. It returns whether root changed,
// or an error when the scoped value cannot be encoded.
func syncResponsePromptCacheKey(root map[string]json.RawMessage, key relaymodel.PromptCacheKey) (bool, error) {
	existing, exists := root[promptCacheKeyRootKey]
	forwarded := key.Forwarded()
	if forwarded == "" {
		if exists {
			delete(root, promptCacheKeyRootKey)
			return true, nil
		}
		return false, nil
	}
	encoded, err := json.Marshal(forwarded)
	if err != nil {
		return false, errors.Wrap(err, "encode scoped prompt_cache_key")
	}
	if exists && bytes.Equal(existing, encoded) {
		return false, nil
	}
	root[promptCacheKeyRootKey] = encoded
	return true, nil
}
