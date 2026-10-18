package otelbridge

import (
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
)

// DeriveLogExceptionAttributes derives only missing exception metadata with the
// pinned SDK's error-type semantics. encodedMessage, when nonnil, is a message
// already encoded by Zap and avoids evaluating Error a second time. Explicit
// attributes take precedence, including empty values. Callback panics or invalid
// text reject the entire derivation without returning partially derived fields.
func DeriveLogExceptionAttributes(err error, attrs []attribute.KeyValue, encodedMessage *string) (derived []attribute.KeyValue, failure error) {
	defer func() {
		if recover() != nil {
			derived = nil
			failure = ErrLogEncoding
		}
	}()
	if err == nil {
		return nil, nil
	}
	var hasMessage, hasType bool
	for _, kv := range attrs {
		switch kv.Key {
		case "exception.message":
			hasMessage = true
		case "exception.type":
			hasType = true
		}
	}
	if !hasMessage {
		var message string
		if encodedMessage != nil {
			message = *encodedMessage
		} else {
			message = err.Error()
		}
		if !utf8.ValidString(message) {
			return nil, ErrLogEncoding
		}
		if message != "" {
			derived = append(derived, attribute.String("exception.message", message))
		}
	}
	if !hasType {
		kind := sdkLogErrorType(err)
		if !utf8.ValidString(kind) {
			return nil, ErrLogEncoding
		}
		if kind != "" {
			derived = append(derived, attribute.String("exception.type", kind))
		}
	}
	return derived, nil
}

var logFmtWrapErrorType = reflect.TypeOf(fmt.Errorf("wrapped: %w", errors.New("err")))

// sdkLogErrorType matches sdk/log v0.22.0 errorType, including its deliberately
// narrow fmt single-wrapper unwrapping and named-versus-pointer type projection.
func sdkLogErrorType(err error) string {
	if typed, ok := err.(interface{ ErrorType() string }); ok {
		if kind := typed.ErrorType(); kind != "" {
			return kind
		}
	}
	for reflect.TypeOf(err) == logFmtWrapErrorType {
		inner := errors.Unwrap(err)
		if inner == nil {
			break
		}
		err = inner
	}
	typ := reflect.TypeOf(err)
	if typ == nil {
		return ""
	}
	pkg, name := typ.PkgPath(), typ.Name()
	if pkg != "" && name != "" {
		return pkg + "." + name
	}
	return typ.String()
}
