package otelbridge

import (
	"reflect"
	"strings"
	"unicode"
)

// jsonField retains the JSON-visible field path and dominance metadata. This
// follows encoding/json's shortest-depth, tagged-field tie-breaking rules.
type jsonField struct {
	name              string
	index             []int
	tagged, omitEmpty bool
}

// jsonVisibleFields examines type metadata only. It never invokes a marshaler,
// IsZero method, or arbitrary application callback a second time.
func jsonVisibleFields(root reflect.Type) []jsonField {
	candidates := map[string][]jsonField{}
	var walk func(reflect.Type, []int, map[reflect.Type]bool)
	walk = func(typ reflect.Type, path []int, seen map[reflect.Type]bool) {
		if len(path) > maxLogEncodingDepth || seen[typ] {
			return
		}
		next := make(map[reflect.Type]bool, len(seen)+1)
		for k, v := range seen {
			next[k] = v
		}
		next[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			ft := field.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if field.PkgPath != "" && (!field.Anonymous || ft.Kind() != reflect.Struct) {
				continue
			}
			tag := field.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, options, _ := strings.Cut(tag, ",")
			if !validJSONTag(name) {
				name = ""
			}
			index := append(append([]int(nil), path...), i)
			if name == "" && field.Anonymous && ft.Kind() == reflect.Struct {
				walk(ft, index, next)
				continue
			}
			tagged := name != ""
			if name == "" {
				name = field.Name
			}
			candidates[name] = append(candidates[name], jsonField{name: name, index: index, tagged: tagged, omitEmpty: hasJSONOption(options, "omitempty")})
		}
	}
	walk(root, nil, nil)
	var fields []jsonField
	for _, group := range candidates {
		shortest := len(group[0].index)
		for _, f := range group {
			if len(f.index) < shortest {
				shortest = len(f.index)
			}
		}
		var eligible []jsonField
		tagged := false
		for _, f := range group {
			if len(f.index) != shortest {
				continue
			}
			if f.tagged && !tagged {
				eligible = nil
				tagged = true
			}
			if f.tagged == tagged {
				eligible = append(eligible, f)
			}
		}
		if len(eligible) == 1 {
			fields = append(fields, eligible[0])
		}
	}
	return fields
}

// reflectedField skips nil anonymous pointers exactly as JSON does.
func reflectedField(value reflect.Value, path []int) (reflect.Value, bool) {
	for _, index := range path {
		if value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		value = value.Field(index)
	}
	return value, true
}

func hasJSONOption(options, want string) bool {
	for _, option := range strings.Split(options, ",") {
		if option == want {
			return true
		}
	}
	return false
}

// validJSONTag mirrors encoding/json's accepted punctuation and Unicode names.
func validJSONTag(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c) {
			continue
		}
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

// jsonEmptyValue matches the ordinary omitempty rule without callbacks.
func jsonEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}
