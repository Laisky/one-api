package controller

import relaymodel "github.com/Laisky/one-api/relay/model"

// sanitizeConvertedChatFields returns a copy-on-write shared Chat wire DTO with
// Claude-only controls removed. Provider-specific Claude DTOs and explicit raw
// passthrough requests are not changed. Portable thinking type and budget survive.
func sanitizeConvertedChatFields(converted any) any {
	switch request := converted.(type) {
	case *relaymodel.GeneralOpenAIRequest:
		if request == nil {
			return request
		}
		copy := *request
		copy.OutputConfig = nil
		if request.Thinking != nil && (len(request.Thinking.ExtraFields) > 0 || request.Thinking.BlockBinding != nil) {
			thinking := *request.Thinking
			thinking.ExtraFields = nil
			thinking.BlockBinding = nil
			copy.Thinking = &thinking
		}
		return &copy
	case relaymodel.GeneralOpenAIRequest:
		return *sanitizeConvertedChatFields(&request).(*relaymodel.GeneralOpenAIRequest)
	default:
		return converted
	}
}
