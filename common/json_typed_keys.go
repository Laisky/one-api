package common

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/Laisky/errors/v2"
)

// ErrAmbiguousJSONKey marks a JSON request whose root object names a typed
// parameter in a way decoders disagree on: more than once (after escape
// decoding and case folding), or under a case-folded spelling of its tagged
// name. encoding/json folds key case and keeps the last duplicate, while
// providers that receive the raw bytes read exact keys (often the first), so
// the gateway would authorize, route and bill a value the upstream never
// applies. Callers map it to HTTP 400.
var ErrAmbiguousJSONKey = errors.New("ambiguous JSON request parameter")

// maxReportedJSONKeys bounds how many offending keys one error lists.
const maxReportedJSONKeys = 5

// jsonFieldIndex describes the root members encoding/json decodes into one
// struct type.
type jsonFieldIndex struct {
	// names maps a folded member name to the exact field names sharing it.
	names map[string][]string
	// tagged marks folded names declared by an explicit json tag; those have
	// a canonical wire spelling that every client and provider uses verbatim.
	tagged map[string]bool
}

// jsonFieldIndexCache memoizes jsonFieldIndexFor per struct type.
var jsonFieldIndexCache sync.Map // map[reflect.Type]*jsonFieldIndex

// jsonFieldIndexFor returns the cached member index of the struct behind t.
// Parameters: t is a decode target type; pointers are dereferenced. Returns:
// the index, or nil when t is not a struct and so has no typed members.
func jsonFieldIndexFor(t reflect.Type) *jsonFieldIndex {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	if cached, ok := jsonFieldIndexCache.Load(t); ok {
		return cached.(*jsonFieldIndex)
	}
	index := &jsonFieldIndex{names: map[string][]string{}, tagged: map[string]bool{}}
	collectJSONFields(t, index, map[reflect.Type]bool{})
	actual, _ := jsonFieldIndexCache.LoadOrStore(t, index)
	return actual.(*jsonFieldIndex)
}

// collectJSONFields walks struct fields the way encoding/json resolves member
// names, including promoted fields of embedded structs. Parameters: t is a
// struct type; index receives the members; visiting guards against recursive
// embedding. Returns: none.
func collectJSONFields(t reflect.Type, index *jsonFieldIndex, visiting map[reflect.Type]bool) {
	if visiting[t] {
		return
	}
	visiting[t] = true
	defer delete(visiting, t)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		// Embedded structs of unexported types still contribute their
		// exported fields; other unexported fields are never decoded.
		if field.Anonymous {
			if !field.IsExported() && fieldType.Kind() != reflect.Struct {
				continue
			}
		} else if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" && field.Anonymous && fieldType.Kind() == reflect.Struct {
			collectJSONFields(fieldType, index, visiting)
			continue
		}
		tagged := name != ""
		if !tagged {
			name = field.Name
		}
		fold := FoldJSONKey(name)
		if !slices.Contains(index.names[fold], name) {
			index.names[fold] = append(index.names[fold], name)
		}
		index.tagged[fold] = index.tagged[fold] || tagged
	}
}

// JSONFieldFolds indexes the JSON member names that encoding/json decodes
// into fields of struct type t by their fold. Pointer types are dereferenced;
// non-struct types have no typed members.
// Parameters: t is the decode target type. Returns: a new fold -> canonical
// name map, or nil for non-struct types.
func JSONFieldFolds(t reflect.Type) map[string]string {
	index := jsonFieldIndexFor(t)
	if index == nil {
		return nil
	}
	folds := make(map[string]string, len(index.names))
	for fold, names := range index.names {
		folds[fold] = names[0]
	}
	return folds
}

// ValidateUnambiguousJSONRootKeys rejects a JSON object payload whose root
// names a field of v's struct type more than once (for example "model" and
// "Model", or a duplicated "n") or spells a tagged field in non-canonical
// case (for example "Max_Tokens"). Payloads whose root is not an object, and
// targets that are not structs, have no typed root members and are accepted.
// Parameters: body is the raw JSON payload; v is the decode target. Returns:
// nil, an error wrapping ErrAmbiguousJSONKey that names the parameters, or an
// error for invalid JSON.
func ValidateUnambiguousJSONRootKeys(body []byte, v any) error {
	if !json.Valid(body) {
		return errors.New("invalid JSON payload")
	}
	return validateDecodedJSONRootKeys(body, v)
}

// validateDecodedJSONRootKeys is ValidateUnambiguousJSONRootKeys for a body
// that json.Unmarshal has already accepted, skipping a second validation pass.
// Parameters: body is valid JSON; v is the decode target. Returns: nil or an
// error wrapping ErrAmbiguousJSONKey.
func validateDecodedJSONRootKeys(body []byte, v any) error {
	index := jsonFieldIndexFor(reflect.TypeOf(v))
	if index == nil || len(index.names) == 0 {
		return nil
	}
	keys, err := scanValidJSONRootKeys(body)
	if err != nil {
		if errors.Is(err, errJSONRootNotObject) {
			return nil
		}
		return errors.Wrap(err, "scan JSON root keys")
	}

	seen := make(map[string]int, len(keys))
	var problems []string
	for _, key := range keys {
		fold := FoldJSONKey(key)
		names, typed := index.names[fold]
		if !typed {
			continue
		}
		seen[fold]++
		if seen[fold] == 2 {
			problems = append(problems, fmt.Sprintf("parameter %q is sent more than once", names[0]))
		}
		if index.tagged[fold] && !slices.Contains(names, key) {
			// Case folding maps rune to rune, so key is as short as names[0].
			problems = append(problems, fmt.Sprintf("parameter %q must be spelled %q", key, names[0]))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	if len(problems) > maxReportedJSONKeys {
		problems = append(problems[:maxReportedJSONKeys], "...")
	}
	return errors.Wrapf(ErrAmbiguousJSONKey, "%s (JSON keys are case-sensitive)", strings.Join(problems, "; "))
}
