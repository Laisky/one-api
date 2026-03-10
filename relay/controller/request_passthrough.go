package controller

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
)

var allowedExtraBodyKeys = map[string]struct{}{
	"add_generation_prompt":         {},
	"add_special_tokens":            {},
	"allowed_token_ids":             {},
	"bad_words":                     {},
	"cache_salt":                    {},
	"chat_template":                 {},
	"chat_template_kwargs":          {},
	"continue_final_message":        {},
	"documents":                     {},
	"echo":                          {},
	"enable_response_messages":      {},
	"ignore_eos":                    {},
	"include_stop_str_in_output":    {},
	"kv_transfer_params":            {},
	"length_penalty":                {},
	"media_io_kwargs":               {},
	"min_p":                         {},
	"min_tokens":                    {},
	"mm_processor_kwargs":           {},
	"previous_input_messages":       {},
	"priority":                      {},
	"prompt_logprobs":               {},
	"repetition_detection":          {},
	"repetition_penalty":            {},
	"request_id":                    {},
	"return_token_ids":              {},
	"return_tokens_as_token_ids":    {},
	"skip_special_tokens":           {},
	"spaces_between_special_tokens": {},
	"stop_token_ids":                {},
	"structured_outputs":            {},
	"top_k":                         {},
	"truncate_prompt_tokens":        {},
	"use_beam_search":               {},
	"vllm_xargs":                    {},
}

// passthroughMergeStats captures non-sensitive diagnostics for controlled passthrough merges.
type passthroughMergeStats struct {
	UnknownPreserved     int
	AllowedRootPreserved int
	ExtraBodyMerged      int
	ExtraBodySkipped     int
	ExtraBodyRejected    int
}

// mergeControlledPassthroughJSON merges allowlisted passthrough fields from the
// raw request into an updated payload without letting nested extra_body override
// explicit top-level request fields.
func mergeControlledPassthroughJSON(original, updated []byte, allowUnknown bool) ([]byte, passthroughMergeStats, bool, error) {
	var stats passthroughMergeStats

	if len(updated) == 0 {
		return original, stats, false, nil
	}

	updatedMap, err := unmarshalJSONMap(updated)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "unmarshal updated payload")
	}

	originalMap, err := unmarshalJSONMap(original)
	if err != nil {
		originalMap = map[string]json.RawMessage{}
	}

	changed := false
	if _, ok := updatedMap["extra_body"]; ok {
		delete(updatedMap, "extra_body")
		changed = true
	}

	if allowUnknown {
		for key, value := range originalMap {
			if key == "extra_body" || isAllowedExtraBodyKey(key) {
				continue
			}
			if _, exists := updatedMap[key]; exists {
				continue
			}
			updatedMap[key] = value
			stats.UnknownPreserved++
			changed = true
		}
	}

	for key, value := range originalMap {
		if !isAllowedExtraBodyKey(key) {
			continue
		}
		if _, exists := updatedMap[key]; exists {
			continue
		}
		updatedMap[key] = value
		stats.AllowedRootPreserved++
		changed = true
	}

	combinedExtraBody, rejected := collectCombinedExtraBody(originalMap, updatedMap)
	stats.ExtraBodyRejected += rejected
	for key, value := range combinedExtraBody {
		if !isAllowedExtraBodyKey(key) {
			stats.ExtraBodyRejected++
			continue
		}
		if _, exists := updatedMap[key]; exists {
			stats.ExtraBodySkipped++
			continue
		}
		updatedMap[key] = value
		stats.ExtraBodyMerged++
		changed = true
	}

	if !changed {
		return updated, stats, false, nil
	}

	merged, err := json.Marshal(updatedMap)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "marshal merged payload")
	}

	return merged, stats, !bytes.Equal(updated, merged), nil
}

// hasPassthroughDiagnostics reports whether any controlled passthrough activity occurred.
func hasPassthroughDiagnostics(stats passthroughMergeStats) bool {
	return stats.UnknownPreserved > 0 ||
		stats.AllowedRootPreserved > 0 ||
		stats.ExtraBodyMerged > 0 ||
		stats.ExtraBodySkipped > 0 ||
		stats.ExtraBodyRejected > 0
}

// collectCombinedExtraBody merges raw and typed extra_body maps, prioritizing raw
// request values so explicit caller settings win over server-added defaults.
func collectCombinedExtraBody(originalMap, updatedMap map[string]json.RawMessage) (map[string]json.RawMessage, int) {
	combined := map[string]json.RawMessage{}
	rejected := 0

	for _, source := range []map[string]json.RawMessage{originalMap, updatedMap} {
		rawExtra, ok := source["extra_body"]
		if !ok || len(rawExtra) == 0 {
			continue
		}

		extraBody, ok := decodeRawMessageMap(rawExtra)
		if !ok {
			rejected++
			continue
		}

		for key, value := range extraBody {
			normalizedKey := strings.TrimSpace(key)
			if normalizedKey == "" {
				rejected++
				continue
			}
			if _, exists := combined[normalizedKey]; exists {
				continue
			}
			combined[normalizedKey] = value
		}
	}

	return combined, rejected
}

// decodeRawMessageMap unmarshals a raw JSON object into a RawMessage map.
func decodeRawMessageMap(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, false
	}

	return data, true
}

// isAllowedExtraBodyKey reports whether a passthrough key is explicitly allowed.
func isAllowedExtraBodyKey(key string) bool {
	_, ok := allowedExtraBodyKeys[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

// unmarshalJSONMap decodes a JSON object into a raw-message map.
func unmarshalJSONMap(payload []byte) (map[string]json.RawMessage, error) {
	if len(payload) == 0 {
		return map[string]json.RawMessage{}, nil
	}

	var out map[string]json.RawMessage
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, err
	}

	if out == nil {
		return map[string]json.RawMessage{}, nil
	}

	return out, nil
}
