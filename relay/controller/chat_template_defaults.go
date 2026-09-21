package controller

import "encoding/json"

// mergeChatTemplateDefaults adds missing keys without overwriting an explicit
// template setting. Parameters: preferred is the caller/root object and defaults
// is a lower-precedence object. Returns: their raw-JSON merge and whether a key
// was added. Invalid objects remain unchanged; opaque numbers retain precision.
func mergeChatTemplateDefaults(preferred, defaults json.RawMessage) (json.RawMessage, bool) {
	caller, callerOK := decodeRawMessageMap(preferred)
	fallback, fallbackOK := decodeRawMessageMap(defaults)
	if !callerOK || !fallbackOK || caller == nil || fallback == nil {
		return preferred, false
	}
	changed := false
	for key, value := range fallback {
		if _, explicit := caller[key]; !explicit {
			caller[key] = value
			changed = true
		}
	}
	if !changed {
		return preferred, false
	}
	merged, err := json.Marshal(caller)
	if err != nil {
		// Both objects contain only already-validated RawMessages. Preserve
		// the explicit caller object if this invariant ever changes.
		return preferred, false
	}
	return merged, true
}
