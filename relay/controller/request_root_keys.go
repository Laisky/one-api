package controller

import (
	"encoding/json"
	"reflect"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// extraBodyKey is the canonical transport-only key whose members are merged
// into the upstream root under allowedExtraBodyKeys.
const extraBodyKey = "extra_body"

// chatFieldsByFold maps the encoding/json fold of every typed chat request
// field to its canonical JSON name.
var chatFieldsByFold = common.JSONFieldFolds(reflect.TypeOf(relaymodel.GeneralOpenAIRequest{}))

// isExtraBodyKey reports whether a decoded root key is read as extra_body by
// Go's case-insensitive struct decoder. Parameters: key is a decoded member
// name. Returns: true for "extra_body" and every case-folded spelling of it.
func isExtraBodyKey(key string) bool {
	return common.JSONKeyMatches(key, extraBodyKey)
}

// chatRawPassthroughSafe reports whether a chat body may be forwarded
// byte-for-byte on the opt-in raw passthrough branch. The decision is made on
// decoded root keys, never on byte substrings, so JSON escapes cannot hide a
// key. Raw bytes are only equivalent to the typed request when every typed
// parameter is spelled canonically and once and no spelling of extra_body is
// present; otherwise a case-sensitive upstream could act on values the
// gateway never admitted, billed or filtered.
// Parameters: body is the raw request payload. Returns: true when raw
// forwarding is equivalent to the typed request view.
func chatRawPassthroughSafe(body []byte) bool {
	keys, err := common.ScanJSONRootKeys(body)
	if err != nil {
		// Not one JSON object (for example a form-bound body): forward the
		// normalized typed request instead of bytes it cannot vouch for.
		return false
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if isExtraBodyKey(key) {
			return false
		}
		canonical, typed := chatFieldsByFold[common.FoldJSONKey(key)]
		if !typed {
			continue
		}
		if key != canonical {
			return false
		}
		if _, duplicate := seen[canonical]; duplicate {
			return false
		}
		seen[canonical] = struct{}{}
	}
	return true
}

// isNonCanonicalChatFieldSpelling reports whether key is a case-folded
// variant of a typed chat field rather than its canonical spelling. Such keys
// already reached the typed request through Go's case-insensitive decoder, so
// preserving them verbatim would forward a second, unreviewed copy.
// Parameters: key is a raw root key. Returns: true for non-canonical variants.
func isNonCanonicalChatFieldSpelling(key string) bool {
	canonical, typed := chatFieldsByFold[common.FoldJSONKey(key)]
	return typed && key != canonical
}

// canonicalizeExtraBodyKey renames a single case-folded spelling of
// extra_body in a decoded root map to the canonical key so the allowlisted
// merge applies to it instead of forwarding it verbatim.
// Parameters: root is a decoded root map (modified in place). Returns: an
// error wrapping common.ErrAmbiguousJSONKey when more than one spelling is
// present.
func canonicalizeExtraBodyKey(root map[string]json.RawMessage) error {
	var variants []string
	for key := range root {
		if isExtraBodyKey(key) {
			variants = append(variants, key)
		}
	}
	switch len(variants) {
	case 0:
		return nil
	case 1:
		if variants[0] != extraBodyKey {
			root[extraBodyKey] = root[variants[0]]
			delete(root, variants[0])
		}
		return nil
	default:
		return errors.Wrap(common.ErrAmbiguousJSONKey, "parameter extra_body is sent more than once (JSON keys match case-insensitively)")
	}
}
