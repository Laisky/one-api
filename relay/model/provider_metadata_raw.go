package model

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
)

// extraContentKey is the canonical carrier name the gateway emits on every wire format.
const extraContentKey = "extra_content"

// providerMetadataNode describes where carriers live below one JSON object of a wire format.
// Scoping is positional: an extra_content key elsewhere (tool arguments, tool_use input, schema
// properties) is caller data and is never touched.
type providerMetadataNode struct {
	// carrier marks objects whose own extra_content key is a gateway carrier.
	carrier bool
	// children names array-valued keys whose object elements are visited with the child node.
	children map[string]*providerMetadataNode
}

var (
	// chatProviderMetadataRoot covers messages[], messages[].tool_calls[], messages[].content[] and tools[].
	chatProviderMetadataRoot = &providerMetadataNode{children: map[string]*providerMetadataNode{
		"messages": {carrier: true, children: map[string]*providerMetadataNode{
			"tool_calls": {carrier: true},
			"content":    {carrier: true},
		}},
		"tools": {carrier: true},
	}}
	// responsesInputProviderMetadata covers each Responses input item and its content[] parts.
	responsesInputProviderMetadata = &providerMetadataNode{carrier: true, children: map[string]*providerMetadataNode{
		"content": {carrier: true},
	}}
)

// ScopeChatBodyProviderMetadata scopes the carriers of a raw Chat Completions body.
// Parameters: body is the raw request and scope is the destination scope. Returns body itself
// (byte-identical) when nothing changes, otherwise the re-encoded body; the count of scoped
// carriers; and an error when a subtree that may hold a carrier is not valid JSON.
func ScopeChatBodyProviderMetadata(body []byte, scope ProviderMetadataScope) ([]byte, int, error) {
	if scope == ProviderMetadataNative {
		return body, 0, nil
	}
	return scopeRawObject(body, chatProviderMetadataRoot, scope)
}

// StripResponsesInputProviderMetadata removes every carrier from a raw Responses "input" value.
// Native Responses providers define no extra_content field on input items or content parts.
// Parameters: input is the raw value (a string input is returned unchanged). Returns the value
// itself when unchanged, the count of removed carriers, and an error for an invalid subtree.
func StripResponsesInputProviderMetadata(input json.RawMessage) (json.RawMessage, int, error) {
	return scopeRawArray(input, responsesInputProviderMetadata, ProviderMetadataNone)
}

// carrierKeys returns the keys of object that JSON decoding would bind to extra_content: the
// canonical spelling plus case-folded variants, which the typed decoders accept too.
func carrierKeys[V any](object map[string]V) []string {
	var keys []string
	for key := range object {
		if strings.EqualFold(key, extraContentKey) {
			keys = append(keys, key)
		}
	}
	return keys
}

// carrierKey returns the single key an untyped part binds to extra_content, preferring the canonical
// spelling exactly as the typed decoder does.
func carrierKey[V any](object map[string]V) (string, bool) {
	if _, present := object[extraContentKey]; present {
		return extraContentKey, true
	}
	keys := carrierKeys(object)
	if len(keys) == 0 {
		return "", false
	}
	return keys[0], true
}

// mayHoldCarrier is a conservative prefilter: false only when raw cannot contain a carrier key, so
// carrier-free subtrees are never decoded or re-encoded. It matches the key case-insensitively and
// treats any \u00XX escape (which could spell an ASCII key letter) as a possible carrier.
func mayHoldCarrier(raw []byte) bool {
	if bytes.Contains(raw, []byte(`\u00`)) {
		return true
	}
	for i := 0; i+len(extraContentKey) <= len(raw); i++ {
		if raw[i]|0x20 != 'e' {
			continue
		}
		if strings.EqualFold(string(raw[i:i+len(extraContentKey)]), extraContentKey) {
			return true
		}
	}
	return false
}

// scopeRawCarriers scopes the carrier keys of one decoded object in place.
// Returns the number of changed carriers. Non-canonical spellings are never a documented Google
// field, so they are always removed; the canonical key follows scopeRawCarrier.
func scopeRawCarriers(object map[string]json.RawMessage, scope ProviderMetadataScope) (int, error) {
	changed := 0
	for _, key := range carrierKeys(object) {
		if key != extraContentKey {
			delete(object, key)
			changed++
			continue
		}
		scoped, keep, count, err := scopeRawCarrier(object[key], scope)
		if err != nil {
			return 0, err
		}
		if !keep {
			delete(object, key)
		} else if count > 0 {
			object[key] = scoped
		}
		changed += count
	}
	return changed, nil
}

// scopeRawObject scopes one JSON object according to node and returns it unchanged when possible.
func scopeRawObject(raw []byte, node *providerMetadataNode, scope ProviderMetadataScope) ([]byte, int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' || !mayHoldCarrier(trimmed) {
		return raw, 0, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return nil, 0, errors.Wrap(err, "decode provider metadata object")
	}
	changed := 0
	if node.carrier {
		count, err := scopeRawCarriers(object, scope)
		if err != nil {
			return nil, 0, err
		}
		changed += count
	}
	for key, child := range node.children {
		value, present := object[key]
		if !present {
			continue
		}
		scoped, count, err := scopeRawArray(value, child, scope)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "scope %s", key)
		}
		if count > 0 {
			object[key] = scoped
			changed += count
		}
	}
	if changed == 0 {
		return raw, 0, nil
	}
	encoded, err := encodeRawJSONValue(object)
	if err != nil {
		return nil, 0, err
	}
	return encoded, changed, nil
}

// scopeRawArray scopes each object element of a JSON array; non-array values are returned unchanged.
func scopeRawArray(raw []byte, node *providerMetadataNode, scope ProviderMetadataScope) ([]byte, int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' || !mayHoldCarrier(trimmed) {
		return raw, 0, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, 0, errors.Wrap(err, "decode provider metadata array")
	}
	changed := 0
	for i := range items {
		scoped, count, err := scopeRawObject(items[i], node, scope)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "element %d", i)
		}
		if count > 0 {
			items[i] = scoped
			changed += count
		}
	}
	if changed == 0 {
		return raw, 0, nil
	}
	encoded, err := encodeRawJSONValue(items)
	if err != nil {
		return nil, 0, err
	}
	return encoded, changed, nil
}

// scopeRawCarrier scopes one raw carrier value. Returns the replacement, whether the key is kept,
// the number of changed carriers (0 or 1), and an encoding error.
func scopeRawCarrier(value json.RawMessage, scope ProviderMetadataScope) (json.RawMessage, bool, int, error) {
	if scope != ProviderMetadataGoogleWire {
		return nil, false, 1, nil
	}
	var carrier map[string]json.RawMessage
	if json.Unmarshal(value, &carrier) != nil || carrier == nil {
		return nil, false, 1, nil
	}
	google, present := carrier["google"]
	if !present {
		return nil, false, 1, nil
	}
	if len(carrier) == 1 {
		return value, true, 0, nil
	}
	encoded, err := encodeRawJSONValue(map[string]json.RawMessage{"google": google})
	if err != nil {
		return nil, false, 0, err
	}
	return encoded, true, 1, nil
}

// encodeRawJSONValue encodes value without HTML escaping, so untouched raw subtrees keep their
// string bytes and number literals.
func encodeRawJSONValue(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, errors.Wrap(err, "encode provider metadata scope")
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}
