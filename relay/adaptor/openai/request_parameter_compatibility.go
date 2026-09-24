package openai

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// NormalizeModelRequestParameters removes documented unsupported sampling fields
// from an owned wire object using its final, mapped upstream model and effective
// reasoning effort. It returns removed field paths, not values. Unknown models
// are left alone; this is not an allowlist of every parameter or provider.
func NormalizeModelRequestParameters(root map[string]json.RawMessage) []string {
	var modelName string
	if json.Unmarshal(root["model"], &modelName) != nil {
		return nil
	}
	defaultEffort, supportsNone, known := modelReasoningSamplingPolicy(modelName)
	if !known {
		return nil
	}
	return normalizeReasoningParameterMap(root, defaultEffort, supportsNone)
}

// modelReasoningSamplingPolicy returns the reasoning sampling contract from
// the existing catalog. Only exact model IDs and valid dated snapshots inherit
// a policy; an arbitrary name beginning with "o" or "gpt" is not evidence.
func modelReasoningSamplingPolicy(modelName string) (defaultEffort string, supportsNone, known bool) {
	name := strings.ToLower(strings.TrimSpace(modelName))
	cfg, exists := ModelRatios[name]
	if !exists && len(name) > 11 && name[len(name)-11] == '-' {
		if _, err := time.Parse("2006-01-02", name[len(name)-10:]); err == nil {
			cfg, exists = ModelRatios[name[:len(name)-11]]
		}
	}
	if !exists || (!slices.Contains(cfg.SupportedFeatures, "reasoning") &&
		len(cfg.SupportedReasoningEfforts) == 0 && cfg.DefaultReasoningEffort == "") {
		return "", false, false
	}
	defaultEffort = cfg.DefaultReasoningEffort
	if defaultEffort == "" {
		defaultEffort = "medium"
	}
	return defaultEffort, slices.Contains(cfg.SupportedReasoningEfforts, "none"), true
}

// modelSupportsSampling reports whether a known model accepts sampling at the
// already-normalized effort. Unknown model contracts retain caller parameters.
func modelSupportsSampling(modelName string, effort *string) bool {
	defaultEffort, supportsNone, known := modelReasoningSamplingPolicy(modelName)
	if !known {
		return true
	}
	if effort != nil && strings.TrimSpace(*effort) != "" {
		defaultEffort = strings.ToLower(strings.TrimSpace(*effort))
	}
	return supportsNone && defaultEffort == "none"
}
