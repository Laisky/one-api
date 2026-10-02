package openai

import (
	"bytes"
	"encoding/json"
	"strings"
)

// normalizeReasoningParameterMap removes only documented sampling controls
// from a caller-owned wire object when reasoning is active. defaultEffort and
// supportsNone come from the selected upstream model's catalog entry. The
// returned paths contain no request values and are safe for diagnostics.
//
// Source: https://developers.openai.com/api/docs/guides/latest-model
// Verified 2026-09-24. Unknown extension values remain raw JSON, not float64.
func normalizeReasoningParameterMap(root map[string]json.RawMessage, defaultEffort string, supportsNone bool) []string {
	effort := defaultEffort
	var explicit string
	if _, chat := root["messages"]; chat {
		_ = json.Unmarshal(root["reasoning_effort"], &explicit)
	} else {
		var reasoning map[string]json.RawMessage
		if json.Unmarshal(root["reasoning"], &reasoning) == nil {
			_ = json.Unmarshal(reasoning["effort"], &explicit)
		}
	}
	if explicit = strings.ToLower(strings.TrimSpace(explicit)); explicit != "" {
		effort = explicit
	}
	if supportsNone && effort == "none" {
		return nil
	}

	var removed []string
	for _, key := range []string{"temperature", "top_p", "logprobs", "top_logprobs"} {
		if _, present := root[key]; present {
			delete(root, key)
			removed = append(removed, key)
		}
	}

	// Do not discard unrelated include entries (especially encrypted reasoning
	// required by stateless continuations), or mutate their shared backing bytes.
	var include []json.RawMessage
	if json.Unmarshal(root["include"], &include) == nil {
		kept := make([]json.RawMessage, 0, len(include))
		for _, item := range include {
			var name string
			if json.Unmarshal(item, &name) == nil && name == "message.output_text.logprobs" {
				continue
			}
			kept = append(kept, item)
		}
		if len(kept) != len(include) {
			if len(kept) == 0 {
				delete(root, "include")
			} else {
				// All items were already validated by json.Unmarshal. Joining raw values
				// preserves unknown numeric tokens exactly and cannot introduce JSON errors.
				var encoded bytes.Buffer
				encoded.WriteByte('[')
				for i, item := range kept {
					if i > 0 {
						encoded.WriteByte(',')
					}
					encoded.Write(item)
				}
				encoded.WriteByte(']')
				root["include"] = encoded.Bytes()
			}
			removed = append(removed, "include.message.output_text.logprobs")
		}
	}
	return removed
}
