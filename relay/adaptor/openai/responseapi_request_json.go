package openai

import (
	"encoding/json"
	"slices"

	"github.com/Laisky/errors/v2"
)

// MarshalJSON serializes a ResponseAPIRequest with model-compatible reasoning
// and sampling parameters. It returns the encoded JSON payload or the
// serialization error without mutating the request or its shared slices.
func (request ResponseAPIRequest) MarshalJSON() ([]byte, error) {
	type responseAPIRequestAlias ResponseAPIRequest

	normalized := responseAPIRequestAlias(request)
	if request.Reasoning != nil && request.Reasoning.Effort != nil && *request.Reasoning.Effort != "" {
		reasoning := *request.Reasoning
		reasoning.Effort = normalizeReasoningEffortForModel(request.Model, request.Reasoning.Effort)
		normalized.Reasoning = &reasoning
	}

	var effort *string
	if normalized.Reasoning != nil {
		effort = normalized.Reasoning.Effort
	}
	if !modelSupportsSampling(request.Model, effort) {
		normalized.Temperature = nil
		normalized.TopP = nil
		normalized.Include = slices.DeleteFunc(slices.Clone(request.Include), func(value string) bool {
			return value == "message.output_text.logprobs"
		})
	}

	encoded, err := json.Marshal(normalized)
	return encoded, errors.WithStack(err)
}
