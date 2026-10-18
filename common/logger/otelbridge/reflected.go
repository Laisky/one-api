package otelbridge

import (
	"bytes"
	"encoding"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
)

// reflectedLogValue follows Zap's JSON reflection contract, including tags and
// explicit MarshalJSON methods. Encoding errors never become invented log text.
func reflectedLogValue(value any) (attribute.Value, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return attribute.Value{}, ErrLogEncoding
	}
	if !utf8.Valid(encoded) || !validJSONSurrogates(encoded) {
		return attribute.Value{}, ErrLogEncoding
	}
	if err := validateReflectedUTF8(reflect.ValueOf(value), encoded, 0); err != nil {
		return attribute.Value{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	decoded, err := decodeJSONLogValue(decoder, 0)
	if err != nil {
		return attribute.Value{}, err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return attribute.Value{}, ErrLogEncoding
	}
	return decoded, nil
}

// jsonLogValue preserves JSON maps/lists/nil and integer precision. OTLP has no
// uint64/big-integer scalar, so out-of-range integer tokens use exact decimal text.
func jsonLogValue(value any, depth int) (attribute.Value, error) {
	if depth > maxLogEncodingDepth {
		return attribute.Value{}, ErrLogEncoding
	}
	switch v := value.(type) {
	case nil:
		return attribute.Value{}, nil
	case string:
		return attribute.StringValue(v), nil
	case bool:
		return attribute.BoolValue(v), nil
	case json.Number:
		return jsonLogNumber(string(v)), nil
	case []any:
		items := make([]attribute.Value, 0, len(v))
		for _, item := range v {
			converted, err := jsonLogValue(item, depth+1)
			if err != nil {
				return attribute.Value{}, err
			}
			items = append(items, converted)
		}
		return attribute.SliceValue(items...), nil
	case map[string]any:
		attrs := make([]attribute.KeyValue, 0, len(v))
		for key, item := range v {
			converted, err := jsonLogValue(item, depth+1)
			if err != nil {
				return attribute.Value{}, err
			}
			attrs = append(attrs, attribute.KeyValue{Key: attribute.Key(key), Value: converted})
		}
		return attribute.MapValue(attrs...), nil
	default:
		return attribute.Value{}, ErrLogEncoding
	}
}

// decodeJSONLogValue retains duplicate keys until explicit protocol validation,
// including custom MarshalJSON output, rather than silently choosing a winner.
func decodeJSONLogValue(d *json.Decoder, depth int) (attribute.Value, error) {
	if depth > maxLogEncodingDepth {
		return attribute.Value{}, ErrLogEncoding
	}
	token, err := d.Token()
	if err != nil {
		return attribute.Value{}, ErrLogEncoding
	}
	if delim, ok := token.(json.Delim); ok {
		switch delim {
		case '{':
			var attrs []attribute.KeyValue
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return attribute.Value{}, ErrLogEncoding
				}
				name, ok := key.(string)
				if !ok {
					return attribute.Value{}, ErrLogEncoding
				}
				value, err := decodeJSONLogValue(d, depth+1)
				if err != nil {
					return attribute.Value{}, err
				}
				attrs = append(attrs, attribute.KeyValue{Key: attribute.Key(name), Value: value})
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return attribute.Value{}, ErrLogEncoding
			}
			return attribute.MapValue(attrs...), nil
		case '[':
			var items []attribute.Value
			for d.More() {
				value, err := decodeJSONLogValue(d, depth+1)
				if err != nil {
					return attribute.Value{}, err
				}
				items = append(items, value)
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return attribute.Value{}, ErrLogEncoding
			}
			return attribute.SliceValue(items...), nil
		default:
			return attribute.Value{}, ErrLogEncoding
		}
	}
	return jsonLogValue(token, depth)
}

// validateReflectedUTF8 detects ordinary reflected invalid strings before JSON's
// replacement behavior. Explicit JSON marshalers define their own projection;
// their encoded result is validated separately without exposing object internals.
func validateReflectedUTF8(v reflect.Value, projection json.RawMessage, depth int) error {
	if depth > maxLogEncodingDepth {
		return ErrLogEncoding
	}
	if !v.IsValid() {
		return nil
	}
	if v.CanInterface() {
		if _, ok := v.Interface().(json.Marshaler); ok {
			return nil
		}
		if _, ok := v.Interface().(encoding.TextMarshaler); ok {
			return nil
		}
	}
	if v.CanAddr() && v.Addr().CanInterface() {
		if _, ok := v.Addr().Interface().(json.Marshaler); ok {
			return nil
		}
		if _, ok := v.Addr().Interface().(encoding.TextMarshaler); ok {
			return nil
		}
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return ErrLogEncoding
		}
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			return validateReflectedUTF8(v.Elem(), projection, depth+1)
		}
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		// Custom map-key text encoding determines the JSON key/value association.
		// Treat that complete declared JSON projection as opaque; invoking its key
		// callbacks again merely to reconstruct a path would change Zap semantics.
		keyType := v.Type().Key()
		textType := reflect.TypeFor[encoding.TextMarshaler]()
		if keyType.Kind() != reflect.String && keyType.Implements(textType) {
			return nil
		}
		var projected map[string]json.RawMessage
		_ = json.Unmarshal(projection, &projected)
		iter := v.MapRange()
		for iter.Next() {
			if err := validateReflectedUTF8(iter.Key(), nil, depth+1); err != nil {
				return err
			}
			var childProjection json.RawMessage
			key := iter.Key()
			switch key.Kind() {
			case reflect.String:
				childProjection = projected[key.String()]
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				childProjection = projected[strconv.FormatInt(key.Int(), 10)]
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
				childProjection = projected[strconv.FormatUint(key.Uint(), 10)]
			}
			if err := validateReflectedUTF8(iter.Value(), childProjection, depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		// JSON byte slices are declared base64 values, not UTF-8 strings.
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var projected []json.RawMessage
		_ = json.Unmarshal(projection, &projected)
		for i := 0; i < v.Len(); i++ {
			var childProjection json.RawMessage
			if i < len(projected) {
				childProjection = projected[i]
			}
			if err := validateReflectedUTF8(v.Index(i), childProjection, depth+1); err != nil {
				return err
			}
		}
	case reflect.Struct:
		var projected map[string]json.RawMessage
		_ = json.Unmarshal(projection, &projected)
		for _, field := range jsonVisibleFields(v.Type()) {
			child, ok := reflectedField(v, field.index)
			childProjection, included := projected[field.name]
			if !ok || projection != nil && !included || projection == nil && field.omitEmpty && jsonEmptyValue(child) {
				continue
			}
			if err := validateReflectedUTF8(child, childProjection, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
