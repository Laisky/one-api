package controller

import (
	"encoding/json"
	"net/http"

	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// validateStoredGoogleMetadata checks automatic replay of stored native Google state.
// Parameters: items are the stored canonical records, binding is optional origin
// qualification, and target is trusted mapped routing. Returns a portability error
// only for opaque Google signatures whose origin cannot be matched.
// Canonical unsigned items, including gateway ordering metadata, remain portable without any binding; ownership and storage
// authorization are unchanged. Explicit stateless caller metadata is not inferred here.
func validateStoredGoogleMetadata(items []state.ItemEnvelope, binding *state.ProviderBinding, target *metalib.Meta) *relaymodel.ErrorWithStatusCode {
	if target == nil || !isNativeGoogleChannel(target) {
		return nil
	}
	for _, item := range items {
		var value any
		if err := json.Unmarshal(item.Raw, &value); err != nil {
			continue
		}
		if !hasStoredGoogleSignature(value) {
			continue
		}
		if binding == nil || binding.ChannelID != target.ChannelId || binding.APIType != target.APIType || binding.ActualModel != target.ActualModelName {
			return stateErrorf(codeStateNotPortable, http.StatusBadRequest, "stored Google state cannot be replayed without the same channel, API and actual model")
		}
	}
	return nil
}

// hasStoredGoogleSignature examines only protocol metadata locations, never schema or tool argument literals.
func hasStoredGoogleSignature(value any) bool {
	fields, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if extra, ok := fields["extra_content"].(map[string]any); ok {
		if google, ok := extra["google"].(map[string]any); ok {
			if signature, ok := google["thought_signature"].(string); ok && signature != "" {
				return true
			}
		}
	}
	for _, key := range []string{"content", "tool_calls"} {
		if children, ok := fields[key].([]any); ok {
			for _, child := range children {
				if hasStoredGoogleSignature(child) {
					return true
				}
			}
		}
	}
	return false
}
