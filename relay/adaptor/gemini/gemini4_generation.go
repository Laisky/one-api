package gemini

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/model"
)

// configureNativeGeneration applies the exact model's published reasoning
// contract to out while preserving other models' existing image controls.
// Parameters: out owns its generation configuration; request is read only and
// names the mapped upstream model. Returns an error for unsupported controls.
func configureNativeGeneration(out *ChatRequest, request model.GeneralOpenAIRequest) error {
	if !geminiOpenaiCompatible.IsGemini4Model(request.Model) {
		return configureNanoBananaThinking(out, request)
	}
	if request.Thinking != nil {
		return errors.New("Gemini 4 requires reasoning effort levels; thinking budgets and provider-specific thinking controls are unsupported")
	}
	requested := request.ReasoningEffort
	if request.Reasoning != nil && request.Reasoning.Effort != nil {
		if requested != nil && !strings.EqualFold(strings.TrimSpace(*requested), strings.TrimSpace(*request.Reasoning.Effort)) {
			return errors.New("conflicting reasoning effort controls")
		}
		requested = request.Reasoning.Effort
	}
	cfg := geminiOpenaiCompatible.ModelRatios[strings.ToLower(request.Model)]
	level, err := resolveNativeThinkingLevel(requested, cfg.SupportedReasoningEfforts, cfg.DefaultReasoningEffort)
	if err != nil {
		return errors.Wrap(err, "validate Gemini 4 reasoning effort")
	}
	// GenerateContent uses uppercase ThinkingLevel enum values; compatible
	// reasoning_effort normalization remains a separate lowercase contract.
	// https://aistudio.google.com/api-reference/generate-content
	out.GenerationConfig.ThinkingConfig = &ThinkingConfig{ThinkingLevel: strings.ToUpper(level)}
	// Gemini 4 ignores these sampling controls. The source request retains them
	// for callers and any later retry through another provider's adaptor.
	out.GenerationConfig.Temperature = nil
	out.GenerationConfig.TopP = nil
	out.GenerationConfig.TopK = 0
	return nil
}
