package adaptor

import (
	"slices"
	"strings"
)

// SupportedReasoningEffort returns the reasoning effort that may be forwarded
// upstream for modelName under the shared catalog policy. Parameters: catalog
// is the provider's model metadata map (for example an adaptor's
// ModelRatios), modelName is the upstream model ID used as the catalog key,
// and effort is the caller's value, which is never mutated. It returns a new
// pointer holding the trimmed, lowercased effort when that value is listed in
// the model's SupportedReasoningEfforts, and nil otherwise: for a nil or blank
// effort, an unknown model, a model that publishes no effort vocabulary, and a
// value the model does not list. Dropping the field lets the provider apply
// its documented default instead of rejecting the request, and the gateway
// never guesses a billing-relevant reasoning depth. Catalog vocabularies are
// stored in their canonical lowercase spelling, so membership is exact after
// normalizing only the caller's value.
func SupportedReasoningEffort(catalog map[string]ModelConfig, modelName string, effort *string) *string {
	if effort == nil {
		return nil
	}
	cfg, ok := catalog[modelName]
	if !ok || len(cfg.SupportedReasoningEfforts) == 0 {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*effort))
	if normalized == "" || !slices.Contains(cfg.SupportedReasoningEfforts, normalized) {
		return nil
	}
	return &normalized
}
