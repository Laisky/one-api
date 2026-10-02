package moonshot

import (
	"slices"
	"strings"

	"github.com/Laisky/one-api/relay/model"
)

// Moonshot pins some OpenAI sampling knobs to a constant value on its newer
// models and answers 400 invalid_request_error when a request carries a
// different value; the vendor guidance is to omit them entirely.
//
//	"表中"固定"表示该参数不可修改：传入其他值会报错，建议不要显式传入。"
//	-- https://platform.kimi.com/docs/api/models-overview
//
// Kimi K3 pins temperature=1.0, top_p=0.95, n=1, presence_penalty=0 and
// frequency_penalty=0. Since an OpenAI-compatible gateway forwards temperature
// on almost every request, leaving them in place would fail nearly all K3
// traffic. Rather than hardcode the model name, we drive the decision off the
// per-model SupportedSamplingParameters metadata: a model that does not
// advertise a knob does not accept it.

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
// Only models that pin sampling wholesale are touched, and "does not advertise
// temperature" is the marker for that: every conventional Moonshot model
// accepts a caller-chosen temperature, so a config omitting it is describing a
// pinned model. Deriving the decision from each parameter's absence instead
// would over-reach, because none of the conventional models advertise "n"
// either yet all of them accept it.
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
// target model supports. Kimi K3 is the only Moonshot model that takes the
// field (always-on thinking, depth selected by a top-level low/high/max), so
// every other model has it dropped exactly as before.
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
