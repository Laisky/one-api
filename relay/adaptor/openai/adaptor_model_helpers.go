package openai

import (
	"maps"
	"strconv"
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

// extractGptVersion returns the numeric GPT major/minor version if present in the model name.
// It expects normalized lowercase model names and only parses names starting with "gpt-".
func extractGptVersion(modelName string) (float64, bool) {
	lower := normalizedModelName(modelName)
	if !strings.HasPrefix(lower, "gpt-") {
		return 0, false
	}

	remainder := strings.TrimPrefix(lower, "gpt-")
	cutIdx := strings.IndexFunc(remainder, func(r rune) bool {
		return !(r == '.' || (r >= '0' && r <= '9'))
	})
	if cutIdx == 0 {
		return 0, false
	}
	if cutIdx > 0 {
		remainder = remainder[:cutIdx]
	}

	version, err := strconv.ParseFloat(remainder, 64)
	if err != nil {
		return 0, false
	}

	return version, true
}

// isModelSupportedReasoning checks if the model supports reasoning features.
func isModelSupportedReasoning(modelName string) bool {
	lower := normalizedModelName(modelName)
	if strings.HasPrefix(lower, "o") {
		return true
	}

	if version, ok := extractGptVersion(lower); ok && version >= 5 {
		if strings.HasPrefix(lower, "gpt-5-chat-latest") || lower == "gpt-5-chat" {
			return false
		}
		return true
	}

	return false
}

// isWebSearchModel returns true when the upstream OpenAI model uses the web search surface.
func isWebSearchModel(modelName string) bool {
	return strings.Contains(modelName, "-search") || strings.Contains(modelName, "-search-preview")
}

func isDeepResearchModel(modelName string) bool {
	return strings.Contains(modelName, "deep-research")
}

// isMediumOnlyReasoningModel reports whether the model only accepts a single
// "medium" reasoning_effort level. The data-driven check examines
// SupportedReasoningEfforts on the ModelConfig; when the model is unknown the
// helper falls back to name-based heuristics for forward compatibility.
func isMediumOnlyReasoningModel(modelName string) bool {
	lower := normalizedModelName(modelName)
	if lower == "" {
		return false
	}

	if cfg, ok := ModelRatios[modelName]; ok && len(cfg.SupportedReasoningEfforts) > 0 {
		if len(cfg.SupportedReasoningEfforts) != 1 {
			return false
		}
		return strings.EqualFold(cfg.SupportedReasoningEfforts[0], "medium")
	}

	if strings.HasPrefix(lower, "o") {
		return true
	}

	if version, ok := extractGptVersion(lower); ok && version >= 5 {
		if strings.Contains(lower, "-chat") {
			return true
		}
	}

	return false
}

// defaultReasoningEffortForModel returns the default reasoning effort level
// for the model. Falls back to "medium" when the model config doesn't pin one.
func defaultReasoningEffortForModel(modelName string) string {
	if cfg, ok := ModelRatios[modelName]; ok {
		if cfg.DefaultReasoningEffort != "" {
			return cfg.DefaultReasoningEffort
		}
		if len(cfg.SupportedReasoningEfforts) == 1 {
			return cfg.SupportedReasoningEfforts[0]
		}
	}
	return "medium"
}

// isReasoningEffortAllowedForModel reports whether `effort` is a permitted
// reasoning_effort value for the model. Data-driven against the model config
// when populated; falls back to name-based heuristics otherwise.
func isReasoningEffortAllowedForModel(modelName, effort string) bool {
	if effort == "" {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(effort))

	if cfg, ok := ModelRatios[modelName]; ok && len(cfg.SupportedReasoningEfforts) > 0 {
		for _, allowed := range cfg.SupportedReasoningEfforts {
			if strings.EqualFold(allowed, normalized) {
				return true
			}
		}
		return false
	}

	if isDeepResearchModel(modelName) || isMediumOnlyReasoningModel(modelName) {
		return normalized == "medium"
	}
	switch normalized {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func normalizeReasoningEffortForModel(modelName string, effort *string) *string {
	defaultEffort := defaultReasoningEffortForModel(modelName)
	if effort == nil {
		return stringRef(defaultEffort)
	}
	normalized := strings.ToLower(strings.TrimSpace(*effort))
	if !isReasoningEffortAllowedForModel(modelName, normalized) {
		return stringRef(defaultEffort)
	}
	return stringRef(normalized)
}

func stringRef(value string) *string {
	return &value
}

func ensureWebSearchTool(request *model.GeneralOpenAIRequest) {
	for _, tool := range request.Tools {
		if strings.EqualFold(tool.Type, "web_search") {
			return
		}
	}

	request.Tools = append(request.Tools, model.Tool{Type: "web_search"})
}

// normalizeClaudeToolChoice coerces Claude tool_choice payloads into the ChatCompletion
// format that upstream OpenAI-compatible adapters expect (type=function with function name).
func normalizeClaudeToolChoice(choice any) any {
	switch src := choice.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(src))
		maps.Copy(cloned, src)

		typeVal, _ := cloned["type"].(string)
		switch strings.ToLower(typeVal) {
		case "tool":
			name, _ := cloned["name"].(string)
			var fn map[string]any
			if existing, ok := cloned["function"].(map[string]any); ok {
				fn = cloneMap(existing)
			} else {
				fn = map[string]any{}
			}
			if name != "" && fn["name"] == nil {
				fn["name"] = name
			}
			if len(fn) > 0 {
				cloned["function"] = fn
			}
			cloned["type"] = "function"
			delete(cloned, "name")
		case "function":
			if name, ok := cloned["name"].(string); ok && name != "" {
				fn, _ := cloned["function"].(map[string]any)
				if fn == nil {
					fn = map[string]any{}
				}
				if fn["name"] == nil {
					fn["name"] = name
				}
				cloned["function"] = fn
				delete(cloned, "name")
			}
		}
		return cloned
	default:
		return choice
	}
}

// cloneMap returns a shallow copy of the provided map.
func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	maps.Copy(cloned, input)
	return cloned
}
