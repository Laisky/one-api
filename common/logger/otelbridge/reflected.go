package otelbridge

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
)

// reflectedLogValue follows Zap's JSON reflection contract, including tags and
// explicit MarshalJSON methods. Encoding errors never become invented log text.
func reflectedLogValue(value any) (attribute.Value, error) {
	if native, ok := value.(attribute.Value); ok {
		return native, ValidateLogValue(native)
	}
	if err := validateReflectedUTF8(reflect.ValueOf(value), 0); err != nil {
		return attribute.Value{}, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return attribute.Value{}, ErrLogEncoding
	}
	if !utf8.Valid(encoded) {
		return attribute.Value{}, ErrLogEncoding
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
		if !strings.ContainsAny(string(v), ".eE") {
			n, err := strconv.ParseInt(string(v), 10, 64)
			if err == nil {
				return attribute.Int64Value(n), nil
			}
			return attribute.StringValue(string(v)), nil
		}
		n, err := strconv.ParseFloat(string(v), 64)
		if err != nil {
			return attribute.Value{}, ErrLogEncoding
		}
		return attribute.Float64Value(n), nil
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
func validateReflectedUTF8(v reflect.Value, depth int) error {
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
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return ErrLogEncoding
		}
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			return validateReflectedUTF8(v.Elem(), depth+1)
		}
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		iter := v.MapRange()
		for iter.Next() {
			if err := validateReflectedUTF8(iter.Key(), depth+1); err != nil {
				return err
			}
			if err := validateReflectedUTF8(iter.Value(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		// JSON byte slices are declared base64 values, not UTF-8 strings.
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		for i := 0; i < v.Len(); i++ {
			if err := validateReflectedUTF8(v.Index(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.PkgPath != "" || field.Tag.Get("json") == "-" {
				continue
			}
			if err := validateReflectedUTF8(v.Field(i), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
