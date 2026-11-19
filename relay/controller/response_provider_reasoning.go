package controller

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// normalizeResponseProviderReasoning applies xAI's model effort contract on the
// native Responses path, which bypasses ConvertRequest. Parameters: request is
// mutated and channel identifies the actual provider. Returns: none. Known xAI
// models follow the shared catalog policy used by xAI Chat conversion: only a
// listed effort survives, normalized to its canonical lowercase spelling, and
// any other value is omitted rather than forwarding a globally accepted but
// provider-invalid value. Unknown models and other providers retain their
// explicit fields and opaque extensions.
func normalizeResponseProviderReasoning(request *openai.ResponseAPIRequest, channel int) {
	if channel != channeltype.XAI || request == nil || request.Reasoning == nil || request.Reasoning.Effort == nil {
		return
	}
	if _, known := xai.ModelRatios[request.Model]; known {
		request.Reasoning.Effort = adaptor.SupportedReasoningEffort(xai.ModelRatios, request.Model, request.Reasoning.Effort)
	}
}
