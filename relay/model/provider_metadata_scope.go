package model

import "slices"

// ProviderMetadataScope names which gateway provider-metadata carriers ("extra_content") a
// destination wire accepts. The gateway emits these carriers to clients for Google turns, and
// clients replay them in later requests that may be routed to any provider, so every outbound
// boundary scopes them to its destination instead of forwarding them as unknown fields.
type ProviderMetadataScope int

const (
	// ProviderMetadataNone strips every carrier: the destination wire defines no extra_content
	// field (OpenAI, Anthropic, Mistral, DeepSeek, ...) and strict validators reject it.
	ProviderMetadataNone ProviderMetadataScope = iota
	// ProviderMetadataGoogleWire keeps only the documented extra_content.google namespace for
	// Google's OpenAI-compatible endpoints; the gateway-only oneapi namespace is removed.
	ProviderMetadataGoogleWire
	// ProviderMetadataNative keeps every carrier because a native Google converter consumes
	// them before transport and never serializes them as extra_content.
	ProviderMetadataNative
)

// String returns a stable log label for the scope.
func (s ProviderMetadataScope) String() string {
	switch s {
	case ProviderMetadataGoogleWire:
		return "google_wire"
	case ProviderMetadataNative:
		return "native"
	default:
		return "none"
	}
}

// ScopeExtraContent returns the carrier a destination may receive.
// Parameters: extra is a caller-owned carrier (never mutated) and scope is the destination scope.
// Returns extra itself when nothing changes, a new Google-only carrier, or nil when nothing remains.
func ScopeExtraContent(extra *ToolExtraContent, scope ProviderMetadataScope) *ToolExtraContent {
	switch {
	case extra == nil || scope == ProviderMetadataNative:
		return extra
	case scope != ProviderMetadataGoogleWire || extra.Google == nil:
		return nil
	case extra.OneAPI == nil:
		return extra
	default:
		return &ToolExtraContent{Google: extra.Google}
	}
}

// ScopeChatRequestProviderMetadata limits every Chat carrier (message, tool call, content part and
// tool declaration) of request to scope.
//
// The request itself stays the same object on purpose: adaptors normalize it in place (default output
// limits, history repairs) and billing reads those normalized fields from the caller's pointer, so a
// scoped copy would hide them from quota estimation. Only request.Messages and request.Tools are
// replaced, by clones, when a carrier changes; previously shared slice arrays, content-part maps and
// carrier values are never written through, so any holder of the old slices keeps its view.
// Parameters: request is the destination-bound request and scope is the destination scope.
// Returns the number of scoped carriers; zero means request was not touched.
func ScopeChatRequestProviderMetadata(request *GeneralOpenAIRequest, scope ProviderMetadataScope) int {
	if request == nil || scope == ProviderMetadataNative {
		return 0
	}
	changed := 0
	var messages []Message
	for i := range request.Messages {
		message, count := scopeMessageProviderMetadata(request.Messages[i], scope)
		if count == 0 {
			continue
		}
		if messages == nil {
			messages = slices.Clone(request.Messages)
		}
		messages[i] = message
		changed += count
	}
	var tools []Tool
	for i := range request.Tools {
		scoped := ScopeExtraContent(request.Tools[i].ExtraContent, scope)
		if scoped == request.Tools[i].ExtraContent {
			continue
		}
		if tools == nil {
			tools = slices.Clone(request.Tools)
		}
		tools[i].ExtraContent = scoped
		changed++
	}
	if messages != nil {
		request.Messages = messages
	}
	if tools != nil {
		request.Tools = tools
	}
	return changed
}

// scopeMessageProviderMetadata returns a copy of message with its carriers scoped and the count of
// changed carriers. Slices and part maps are cloned only when one of their carriers changes.
func scopeMessageProviderMetadata(message Message, scope ProviderMetadataScope) (Message, int) {
	changed := 0
	if scoped := ScopeExtraContent(message.ExtraContent, scope); scoped != message.ExtraContent {
		message.ExtraContent = scoped
		changed++
	}
	var calls []Tool
	for i := range message.ToolCalls {
		scoped := ScopeExtraContent(message.ToolCalls[i].ExtraContent, scope)
		if scoped == message.ToolCalls[i].ExtraContent {
			continue
		}
		if calls == nil {
			calls = slices.Clone(message.ToolCalls)
		}
		calls[i].ExtraContent = scoped
		changed++
	}
	if calls != nil {
		message.ToolCalls = calls
	}
	switch content := message.Content.(type) {
	case []MessageContent:
		var parts []MessageContent
		for i := range content {
			scoped := ScopeExtraContent(content[i].ExtraContent, scope)
			if scoped == content[i].ExtraContent {
				continue
			}
			if parts == nil {
				parts = slices.Clone(content)
			}
			parts[i].ExtraContent = scoped
			changed++
		}
		if parts != nil {
			message.Content = parts
		}
	case []any:
		var parts []any
		for i, raw := range content {
			object, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			key, present := carrierKey(object)
			if !present {
				continue
			}
			scoped, keep := scopeDecodedCarrier(object[key], scope)
			if keep && scoped == nil {
				continue
			}
			if parts == nil {
				parts = slices.Clone(content)
			}
			clone := make(map[string]any, len(object))
			for key, field := range object {
				clone[key] = field
			}
			if keep {
				clone[key] = scoped
			} else {
				delete(clone, key)
			}
			parts[i] = clone
			changed++
		}
		if parts != nil {
			message.Content = parts
		}
	}
	return message, changed
}

// scopeDecodedCarrier scopes one decoded extra_content value of an untyped content part.
// Parameters: value is the decoded carrier and scope is not ProviderMetadataNative.
// Returns (nil, true) when the value is already acceptable unchanged, (replacement, true) for a
// Google-only replacement, and (nil, false) when the carrier must be removed.
func scopeDecodedCarrier(value any, scope ProviderMetadataScope) (any, bool) {
	if scope != ProviderMetadataGoogleWire {
		return nil, false
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	google, ok := object["google"]
	if !ok {
		return nil, false
	}
	if len(object) == 1 {
		return nil, true
	}
	return map[string]any{"google": google}, true
}
