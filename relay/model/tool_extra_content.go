package model

// ToolExtraContent carries documented provider metadata on Chat messages, parts and tool calls.
// It is deliberately typed so arbitrary upstream fields do not become gateway controls.
type ToolExtraContent struct {
	OneAPI *OneAPIToolExtraContent `json:"oneapi,omitempty"`
	Google *GoogleToolExtraContent `json:"google,omitempty"`
}

// GoogleToolExtraContent preserves Google's opaque signature and thought-part marker.
type GoogleToolExtraContent struct {
	ThoughtSignature string `json:"thought_signature,omitempty"`
	Thought          bool   `json:"thought,omitempty"`
}

// GoogleThoughtSignature returns the public Google metadata carrier for a signature.
// Parameters: signature is an opaque upstream value. Returns nil for an absent signature.
func GoogleThoughtSignature(signature string) *ToolExtraContent {
	return GooglePartMetadata(signature, false)
}

// GooglePartMetadata returns the public Google metadata carrier for one native part.
// Parameters: signature is opaque and thought marks a thought part. Returns nil when both are absent.
func GooglePartMetadata(signature string, thought bool) *ToolExtraContent {
	if signature == "" && !thought {
		return nil
	}
	return &ToolExtraContent{Google: &GoogleToolExtraContent{ThoughtSignature: signature, Thought: thought}}
}

// ToolThoughtSignature returns a tool call's opaque Google signature or an empty string.
func ToolThoughtSignature(tool Tool) string {
	signature, _ := GoogleMetadata(tool.ExtraContent)
	return signature
}

// GoogleMetadata returns a carrier's signature and thought marker without changing it.
func GoogleMetadata(extra *ToolExtraContent) (string, bool) {
	if extra == nil || extra.Google == nil {
		return "", false
	}
	return extra.Google.ThoughtSignature, extra.Google.Thought
}

// OneAPIToolExtraContent carries gateway metadata, never a Google API field.
type OneAPIToolExtraContent struct {
	GeminiTextPresent *bool `json:"gemini_text_present,omitempty"`
	GeminiPartIndex   *int  `json:"gemini_part_index,omitempty"`
}

// WithGeminiPartIndex returns a copy of extra with one absolute native part position.
// Parameters: extra may be nil and index identifies a part. Returns a new carrier without mutating extra.
func WithGeminiPartIndex(extra *ToolExtraContent, index int) *ToolExtraContent {
	out := ToolExtraContent{}
	if extra != nil {
		out = *extra
	}
	gateway := OneAPIToolExtraContent{}
	if out.OneAPI != nil {
		gateway = *out.OneAPI
	}
	gateway.GeminiPartIndex = &index
	out.OneAPI = &gateway
	return &out
}

// GeminiPartIndex returns a carrier's native part index and whether it was supplied.
func GeminiPartIndex(extra *ToolExtraContent) (int, bool) {
	if extra == nil || extra.OneAPI == nil || extra.OneAPI.GeminiPartIndex == nil {
		return 0, false
	}
	return *extra.OneAPI.GeminiPartIndex, true
}

// WithGeminiTextPresent copies extra and records whether an empty native part had a text oneof.
func WithGeminiTextPresent(extra *ToolExtraContent, present bool) *ToolExtraContent {
	out := ToolExtraContent{}
	if extra != nil {
		out = *extra
	}
	gateway := OneAPIToolExtraContent{}
	if out.OneAPI != nil {
		gateway = *out.OneAPI
	}
	gateway.GeminiTextPresent = &present
	out.OneAPI = &gateway
	return &out
}

// GeminiTextPresent returns the supplied text-presence marker, or true for ordinary public text content.
func GeminiTextPresent(extra *ToolExtraContent) bool {
	return extra == nil || extra.OneAPI == nil || extra.OneAPI.GeminiTextPresent == nil || *extra.OneAPI.GeminiTextPresent
}
