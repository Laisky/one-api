package moonshot

import (
	"slices"
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

// Moonshot fixes sampling values on all current K2.6, K2.7 Code, and K3 models.
// Other values produce invalid_request_error; omit the fixed fields as advised:
// https://platform.kimi.com/docs/api/models-overview
//
// K3 and K2.7 fix temperature=1.0; K2.6 fixes it to 1.0 while thinking and 0.6
// otherwise. All fix top_p=0.95, n=1, and both penalties=0. Metadata determines
// which models need normalization, preserving retained legacy-model behavior.

// pinnedSamplingParams are the knobs Moonshot may pin, in the spelling used by
// ModelConfig.SupportedSamplingParameters. Only these are ever stripped, so a
// model whose metadata simply omits an unrelated parameter keeps forwarding it.
var pinnedSamplingParams = []string{
	"temperature",
	"top_p",
	"n",
	"presence_penalty",
	"frequency_penalty",
}

// openAIToMoonshotEffort maps the OpenAI reasoning_effort ladder onto the
// tiers Kimi K3 accepts (low / high / max). The mapping is monotone and never
// escalates: "medium" becomes "high" rather than falling through to K3's "max"
// default, so a caller asking for less thinking is not silently charged for the
// deepest tier.
var openAIToMoonshotEffort = map[string]string{
	"none":    "low",
	"minimal": "low",
	"low":     "low",
	"medium":  "high",
	"high":    "high",
	"xhigh":   "max",
	"max":     "max",
}

// stripPinnedSamplingParams removes sampling knobs the target model pins to a
// fixed value.
//
// Only models that pin sampling wholesale are touched: their metadata omits
// temperature. Retained legacy models keep their previous forwarding behavior,
// including n even though their historical metadata does not advertise it.
func stripPinnedSamplingParams(request *model.GeneralOpenAIRequest) {
	cfg, ok := ModelRatios[request.Model]
	if !ok || len(cfg.SupportedSamplingParameters) == 0 {
		return
	}
	if slices.Contains(cfg.SupportedSamplingParameters, "temperature") {
		return
	}

	for _, param := range pinnedSamplingParams {
		if slices.Contains(cfg.SupportedSamplingParameters, param) {
			continue
		}

		switch param {
		case "temperature":
			request.Temperature = nil
		case "top_p":
			request.TopP = nil
		case "n":
			request.N = nil
		case "presence_penalty":
			request.PresencePenalty = nil
		case "frequency_penalty":
			request.FrequencyPenalty = nil
		}
	}
}

// normalizeReasoningEffort adapts an incoming reasoning_effort to what the
// target model supports. K3 and K3.1 Pro accept low/high/max; models without
// effort metadata have the field dropped exactly as before.
func normalizeReasoningEffort(request *model.GeneralOpenAIRequest) {
	if request.ReasoningEffort == nil {
		return
	}

	cfg, ok := ModelRatios[request.Model]
	if !ok || len(cfg.SupportedReasoningEfforts) == 0 {
		request.ReasoningEffort = nil
		return
	}

	effort := strings.ToLower(strings.TrimSpace(*request.ReasoningEffort))
	if slices.Contains(cfg.SupportedReasoningEfforts, effort) {
		request.ReasoningEffort = &effort
		return
	}

	mapped, ok := openAIToMoonshotEffort[effort]
	if !ok || !slices.Contains(cfg.SupportedReasoningEfforts, mapped) {
		// Unknown tier: drop it and let the upstream apply its own default
		// rather than guessing a billing-relevant depth.
		request.ReasoningEffort = nil
		return
	}

	request.ReasoningEffort = &mapped
}
