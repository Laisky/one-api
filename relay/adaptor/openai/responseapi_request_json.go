package openai

import "encoding/json"

// MarshalJSON serializes a ResponseAPIRequest and canonicalizes any explicit reasoning effort for the selected model.
// It returns the encoded JSON payload or the serialization error without mutating the request.
func (request ResponseAPIRequest) MarshalJSON() ([]byte, error) {
	type responseAPIRequestAlias ResponseAPIRequest

	normalized := responseAPIRequestAlias(request)
	if request.Reasoning != nil && request.Reasoning.Effort != nil && *request.Reasoning.Effort != "" {
		reasoning := *request.Reasoning
		reasoning.Effort = normalizeReasoningEffortForModel(request.Model, request.Reasoning.Effort)
		normalized.Reasoning = &reasoning
	}

	return json.Marshal(normalized)
}
