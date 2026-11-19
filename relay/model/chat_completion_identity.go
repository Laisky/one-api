package model

import "encoding/json"

// ChatCompletionIdentity preserves the root identity fields of a provider's
// non-streaming Chat Completions body when a handler decodes it into a slim
// struct and re-renders it for the client. OpenAI SDKs expose these fields
// (id, object, created, model, system_fingerprint, service_tier); re-rendering
// only choices and usage dropped them on every Chat-wire route. The values are
// kept as raw JSON so decoding never fails on a provider's unusual scalar type
// and the client receives the provider's exact bytes; absent fields stay
// absent. Embed it without a JSON tag so its fields stay at the root.
type ChatCompletionIdentity struct {
	ResponseID                json.RawMessage `json:"id,omitempty"`
	ResponseObject            json.RawMessage `json:"object,omitempty"`
	ResponseCreated           json.RawMessage `json:"created,omitempty"`
	ResponseModel             json.RawMessage `json:"model,omitempty"`
	ResponseSystemFingerprint json.RawMessage `json:"system_fingerprint,omitempty"`
	ResponseServiceTier       json.RawMessage `json:"service_tier,omitempty"`
}
