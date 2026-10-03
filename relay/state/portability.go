package state

import (
	"encoding/json"
	"strings"
)

// PortabilityClass describes whether a canonical item can be replayed onto a
// different upstream without losing meaning. It drives the portability table in
// Section 5.8 and the state_not_portable error contract.
type PortabilityClass string

const (
	// PortabilityPortable items (plain messages, function calls, function-call
	// outputs, references once resolved) can be lowered onto any target format.
	PortabilityPortable PortabilityClass = "portable"

	// PortabilityProviderBound items (encrypted reasoning, signed thinking, hosted
	// tool-call state) carry opaque provider-owned state. They replay only on a
	// compatible provider binding; otherwise the request fails with
	// state_not_portable rather than being silently degraded.
	PortabilityProviderBound PortabilityClass = "provider_bound"

	// PortabilityDisplayOnly items (readable reasoning summaries) may be shown to a
	// client but must never be promoted into authoritative hidden context.
	PortabilityDisplayOnly PortabilityClass = "display_only"
)

// Item kinds recognized by the ledger. Unknown kinds are retained losslessly and
// classified conservatively as provider-bound so an unknown future item type is
// never silently degraded on a fallback route (I05).
const (
	KindMessage            = "message"
	KindReasoning          = "reasoning"
	KindFunctionCall       = "function_call"
	KindFunctionCallOutput = "function_call_output"
	KindItemReference      = "item_reference"
	KindThinking           = "thinking"
)

// hostedToolCallKinds are provider-hosted/built-in tool items whose state is
// provider-owned (web search, code interpreter, file search, image generation,
// computer use, and MCP calls).
var hostedToolCallKinds = map[string]struct{}{
	"web_search_call":       {},
	"file_search_call":      {},
	"code_interpreter_call": {},
	"image_generation_call": {},
	"computer_call":         {},
	"local_shell_call":      {},
	"mcp_call":              {},
	"mcp_list_tools":        {},
	"mcp_approval_request":  {},
}

// classifyRawItem determines the portability class of a raw canonical item. The
// raw envelope is authoritative, so classification reads only the fields that
// change portability and never rewrites the payload.
func classifyRawItem(kind string, raw json.RawMessage) PortabilityClass {
	kind = strings.ToLower(strings.TrimSpace(kind))

	switch kind {
	case KindMessage, KindFunctionCall, KindFunctionCallOutput, KindItemReference, "":
		return PortabilityPortable
	case KindReasoning:
		// Reasoning is provider-bound only when it carries encrypted state; a
		// summary-only reasoning item is display-only.
		if rawHasNonEmptyField(raw, "encrypted_content") {
			return PortabilityProviderBound
		}
		return PortabilityDisplayOnly
	case KindThinking:
		// Claude signed thinking is provider-bound; unsigned thinking is display.
		if rawHasNonEmptyField(raw, "signature") {
			return PortabilityProviderBound
		}
		return PortabilityDisplayOnly
	}

	if _, ok := hostedToolCallKinds[kind]; ok {
		return PortabilityProviderBound
	}

	// Unknown future item types are retained losslessly but treated as
	// provider-bound so they never degrade silently on an incompatible route.
	return PortabilityProviderBound
}

// Fallback lowering actions describe how a canonical item must be handled when a
// resolved turn is lowered onto a stateless Chat/Claude fallback route.
const (
	// FallbackActionCarry: the item is portable and passes through unchanged.
	FallbackActionCarry = "carry"
	// FallbackActionDrop: the item's opaque provider-bound state has no faithful
	// stateless representation, so it is dropped. Its readable summary (if any)
	// remains display-only and must not be promoted into authoritative context.
	FallbackActionDrop = "drop"
	// FallbackActionFail: the item has no sanctioned stateless degradation and must
	// fail closed with state_not_portable rather than silently corrupt the turn.
	FallbackActionFail = "fail"
)

// FallbackLowering decides how one canonical item is lowered onto a stateless
// Chat or Claude fallback route, implementing the fallback columns of the
// Section 5.8 portability table:
//
//   - portable items (messages, function calls, function-call outputs, and
//     already-resolved references) carry;
//   - reasoning and thinking degrade to a display-only sidecar: their opaque
//     provider-bound state is dropped and the request proceeds;
//   - hosted/built-in tool-call state and unknown future item types have no
//     sanctioned degradation and fail closed with state_not_portable (rows P04,
//     I05, E05).
//
// It returns the action and the parsed item kind (for the error message and
// metrics). It never inspects prompt content.
func FallbackLowering(raw json.RawMessage) (action, kind string) {
	kind = probeItemKind(raw)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case KindMessage, KindFunctionCall, KindFunctionCallOutput, KindItemReference, "":
		return FallbackActionCarry, kind
	case KindReasoning, KindThinking:
		return FallbackActionDrop, kind
	}
	return FallbackActionFail, kind
}

// probeItemKind extracts the "type" field of a raw canonical item, or "" when the
// item is not a JSON object (e.g. a bare string user message).
func probeItemKind(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	return probe.Type
}

// rawHasNonEmptyField reports whether a raw JSON object carries a non-empty
// string value at the given top-level key.
func rawHasNonEmptyField(raw json.RawMessage, field string) bool {
	if len(raw) == 0 {
		return false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	v, ok := probe[field]
	if !ok {
		return false
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return strings.TrimSpace(s) != ""
	}
	// Non-string but present and not null.
	trimmed := strings.TrimSpace(string(v))
	return trimmed != "" && trimmed != "null"
}
