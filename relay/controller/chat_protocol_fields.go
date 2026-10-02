package controller

import (
	"encoding/json"
	"reflect"
	"strings"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

type chatProtocolField struct {
	typ       reflect.Type
	omitEmpty bool
}

var chatProtocolFields = collectChatProtocolFields()

// collectChatProtocolFields derives JSON field metadata once at startup.
// Parameters: none. Returns: immutable metadata for distinguishing converter
// deletions from fields omitted merely because of their original zero value.
func collectChatProtocolFields() map[string]chatProtocolField {
	typ := reflect.TypeOf(relaymodel.GeneralOpenAIRequest{})
	fields := make(map[string]chatProtocolField, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag.Get("json")
		name := strings.SplitN(tag, ",", 2)[0]
		if name != "" && name != "-" {
			fields[name] = chatProtocolField{typ: field.Type, omitEmpty: strings.Contains(tag, ",omitempty")}
		}
	}
	return fields
}

// collectFilteredChatFields identifies known fields removed by conversion.
// Parameters: original and updated are raw payload maps. Returns: removed keys;
// explicit zero values that were already omitted by the DTO are not classified
// as deletions. Only missing known fields are decoded, not the full message body.
func collectFilteredChatFields(original, updated map[string]json.RawMessage) map[string]struct{} {
	removed := map[string]struct{}{}
	for name, raw := range original {
		field, known := chatProtocolFields[name]
		if _, retained := updated[name]; !known || retained {
			continue
		}
		value := reflect.New(field.typ)
		if json.Unmarshal(raw, value.Interface()) != nil {
			continue
		}
		if !field.omitEmpty || !isEmptyChatJSONValue(value.Elem()) {
			removed[name] = struct{}{}
		}
	}
	return removed
}

// isEmptyChatJSONValue mirrors encoding/json's omitempty value classification.
// Parameters: value is a decoded field. Returns: true when its unchanged value
// would be omitted; non-nil pointers remain explicit even when they point to zero.
func isEmptyChatJSONValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return value.IsZero()
	default:
		return false
	}
}
