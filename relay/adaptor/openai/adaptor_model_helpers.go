package openai

import (
	"strconv"
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

func normalizedModelName(modelName string) string {
	return strings.ToLower(strings.TrimSpace(modelName))
}

func modelNameWithoutDateSuffix(modelName string) string {
	const dateSuffixLength = len("-YYYY-MM-DD")
	if len(modelName) < dateSuffixLength {
		return modelName
	}

	index := len(modelName) - dateSuffixLength
	if modelName[index] != '-' || modelName[index+5] != '-' || modelName[index+8] != '-' {
		return modelName
	}

	for _, part := range [][2]int{{index + 1, index + 5}, {index + 6, index + 8}, {index + 9, index + 11}} {
		if _, err := strconv.Atoi(modelName[part[0]:part[1]]); err != nil {
			return modelName
		}
	}

	return modelName[:index]
}

// isModelSupportedReasoning reports whether modelName supports reasoning.
// Catalog metadata takes precedence over name-based compatibility fallbacks.
func isModelSupportedReasoning(modelName string) bool {
	normalizedName := normalizedModelName(modelName)
	if config, ok := ModelRatios[normalizedName]; ok {
		for _, feature := range config.SupportedFeatures {
			if feature == "reasoning" {
				return true
			}
		}
		if len(config.SupportedReasoningEfforts) > 0 || config.DefaultReasoningEffort != "" {
			return true
		}
	}

	switch {
	case strings.HasPrefix(normalizedName, "o1"),
		strings.HasPrefix(normalizedName, "o3"),
		strings.HasPrefix(normalizedName, "o4"),
		strings.HasPrefix(normalizedName, "gpt-5"),
		strings.HasPrefix(normalizedName, "chat-latest"):
		return true
	default:
		return false
	}
}

func isMediumOnlyReasoningModel(modelName string) bool {
	config, ok := ModelRatios[normalizedModelName(modelName)]
	if !ok {
		return false
	}

	if len(config.SupportedReasoningEfforts) != 1 {
		return false
	}
	return config.SupportedReasoningEfforts[0] == "medium"
}

func defaultReasoningEffortForModel(modelName string) string {
	normalizedName := normalizedModelName(modelName)
	if config, ok := ModelRatios[normalizedName]; ok && config.DefaultReasoningEffort != "" {
		return config.DefaultReasoningEffort
	}

	baseModelName := modelNameWithoutDateSuffix(normalizedName)

	switch baseModelName {
	case "o4-mini", "o3":
		return "medium"
	default:
		return "medium"
	}
}

func isReasoningEffortAllowedForModel(modelName, effort string) bool {
	normalizedName := normalizedModelName(modelName)
	config, ok := ModelRatios[normalizedName]
	if ok && len(config.SupportedReasoningEfforts) > 0 {
		for _, supportedEffort := range config.SupportedReasoningEfforts {
			if effort == supportedEffort {
				return true
			}
			if effort == "minimal" && supportedEffort == "none" {
				return true
			}
		}
		return false
	}

	baseModelName := modelNameWithoutDateSuffix(normalizedName)

	if strings.HasPrefix(baseModelName, "o1-pro") {
		return effort == "high"
	}
	if strings.HasPrefix(baseModelName, "o1-mini") || strings.HasPrefix(baseModelName, "o1-preview") {
		return effort == "medium"
	}
	if strings.HasPrefix(baseModelName, "o1") {
		return effort == "low" || effort == "medium" || effort == "high"
	}
	if baseModelName == "o4-mini" || baseModelName == "o3" {
		return effort == "low" || effort == "medium" || effort == "high"
	}

	return true
}

func supportsNoneReasoningEffort(modelName string) bool {
	config, ok := ModelRatios[normalizedModelName(modelName)]
	if !ok {
		return false
	}
	for _, effort := range config.SupportedReasoningEfforts {
		if effort == "none" {
			return true
		}
	}
	return false
}

func normalizeReasoningEffortForModel(modelName string, effort *string) *string {
	normalizedName := normalizedModelName(modelName)
	if isMediumOnlyReasoningModel(normalizedName) {
		effort := "medium"
		return &effort
	}

	if effort == nil || *effort == "" {
		defaultEffort := defaultReasoningEffortForModel(normalizedName)
		return &defaultEffort
	}

	if *effort == "minimal" && supportsNoneReasoningEffort(normalizedName) {
		normalizedEffort := "none"
		return &normalizedEffort
	}

	if isReasoningEffortAllowedForModel(normalizedName, *effort) {
		return effort
	}

	defaultEffort := defaultReasoningEffortForModel(normalizedName)
	return &defaultEffort
}

func standardSamplingParameters() []string {
	return []string{"temperature", "top_p", "max_tokens", "seed", "frequency_penalty", "presence_penalty"}
}

func reasoningSamplingParameters() []string {
	return []string{"seed", "max_tokens"}
}

func containsSupportedParameter(supported []string, parameter string) bool {
	for _, value := range supported {
		if value == parameter {
			return true
		}
	}
	return false
}

func supportsSeedParameter(modelName string) bool {
	config, ok := ModelRatios[normalizedModelName(modelName)]
	if !ok || len(config.SupportedSamplingParameters) == 0 {
		return true
	}
	return containsSupportedParameter(config.SupportedSamplingParameters, "seed")
}

func supportsSamplingParameter(modelName, parameter string) bool {
	config, ok := ModelRatios[normalizedModelName(modelName)]
	if !ok || len(config.SupportedSamplingParameters) == 0 {
		return true
	}
	return containsSupportedParameter(config.SupportedSamplingParameters, parameter)
}

func filterRequestParametersForModel(request *model.GeneralOpenAIRequest, modelName string) {
	if request == nil {
		return
	}

	if !supportsSeedParameter(modelName) {
		request.Seed = nil
	}

	if !supportsSamplingParameter(modelName, "temperature") {
		request.Temperature = nil
	}
	if !supportsSamplingParameter(modelName, "top_p") {
		request.TopP = nil
	}
	if !supportsSamplingParameter(modelName, "frequency_penalty") {
		request.FrequencyPenalty = nil
	}
	if !supportsSamplingParameter(modelName, "presence_penalty") {
		request.PresencePenalty = nil
	}

	if !supportsSamplingParameter(modelName, "max_tokens") {
		request.MaxTokens = 0
		request.MaxCompletionTokens = nil
	}
}
