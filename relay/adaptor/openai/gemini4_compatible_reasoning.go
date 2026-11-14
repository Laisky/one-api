package openai

import (
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/model"
)

// applyGemini4CompatibleReasoning validates the requested effort against the
// registered upstream model and removes ignored sampling controls in place. It
// returns whether a lossless output schema was lowered and any validation error.
// System instructions are preserved.
func applyGemini4CompatibleReasoning(modelName string, request *model.GeneralOpenAIRequest) (bool, error) {
	if request.Thinking != nil {
		return false, errors.Errorf("validation failed: Gemini model %q requires reasoning_effort levels instead of provider-specific thinking controls", modelName)
	}
	requestedEfforts := []*string{request.ReasoningEffort}
	if request.Reasoning != nil {
		requestedEfforts = append(requestedEfforts, request.Reasoning.Effort)
	}
	configEffort, outputFormat, err := decodeGemini4OutputControls(request.OutputConfig, request.ResponseFormat)
	if err != nil {
		return false, errors.Wrap(err, "validate Gemini output controls")
	}
	requestedEfforts = append(requestedEfforts, configEffort)
	if request.Reasoning != nil && request.Reasoning.Summary != nil && strings.TrimSpace(*request.Reasoning.Summary) != "" {
		return false, errors.New("validation failed: reasoning.summary is unsupported by the Gemini Chat compatible contract")
	}

	var effort *string
	for _, requested := range requestedEfforts {
		if requested == nil {
			continue
		}
		normalized := strings.ToLower(strings.TrimSpace(*requested))
		// Google's OpenAI-compatible transport maps minimal to low for Gemini 4,
		// although native thinkingLevel does not accept minimal.
		// https://aistudio.google.com/docs/openai#thinking
		if normalized == "minimal" {
			normalized = "low"
		}
		if normalized == "default" {
			normalized = ""
		}
		if effort != nil && *effort != normalized {
			return false, errors.Errorf("validation failed: conflicting reasoning_effort controls for Gemini model %q", modelName)
		}
		effort = &normalized
	}
	// Omitting the field lets Google select the model-specific default:
	// medium for Argon/Carbon and high for Hydrogen.
	if effort != nil && *effort == "" {
		effort = nil
	}
	if effort != nil {
		config, ok := geminiOpenaiCompatible.ModelRatios[normalizedModelName(modelName)]
		if !ok || !slices.Contains(config.SupportedReasoningEfforts, *effort) {
			return false, errors.Errorf("validation failed: reasoning_effort %q is unsupported by Gemini model %q", *effort, modelName)
		}
	}

	if outputFormat != nil {
		request.ResponseFormat = outputFormat
	}
	request.ReasoningEffort = effort
	request.Reasoning = nil
	request.OutputConfig = nil
	request.Temperature = nil
	request.TopP = nil
	request.TopK = nil
	return outputFormat != nil, nil
}
