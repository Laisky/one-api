package controller

import (
	"slices"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// normalizeResponseProviderReasoning applies xAI's model effort contract on the
// native Responses path, which bypasses ConvertRequest. Parameters: request is
// mutated and channel identifies the actual provider. Returns: none. Unknown
// models and other providers retain their explicit fields and opaque extensions.
func normalizeResponseProviderReasoning(request *openai.ResponseAPIRequest, channel int) {
	if channel != channeltype.XAI || request == nil || request.Reasoning == nil || request.Reasoning.Effort == nil {
		return
	}
	cfg, known := xai.ModelRatios[request.Model]
	if known && !slices.Contains(cfg.SupportedReasoningEfforts, *request.Reasoning.Effort) {
		// Match xAI Chat conversion: omit a model-unsupported effort rather
		// than forwarding a globally accepted but provider-invalid value.
		request.Reasoning.Effort = nil
	}
}
