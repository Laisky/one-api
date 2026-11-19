package model

import (
	"bytes"
	"encoding/json"
	"reflect"

	"github.com/Laisky/errors/v2"
)

// This file holds the JSON decoding of Error and ErrorWithStatusCode. Both are
// only ever decoded from wire payloads that a provider produced, so decoding is
// the trust boundary for provenance (see Error.FromUpstream) and is tolerant of
// optional members whose JSON kind breaks the OpenAI string contract.
//
// Every member decoder follows the json.Unmarshaler convention for repeated
// members. encoding/json matches keys case-insensitively and calls UnmarshalJSON
// once per occurrence of a member, null included, on the same receiver; on Go
// 1.27 (encoding/json backed by json/v2) an error returned by a member decoder
// also does not abort the outer decode. A decoder therefore never returns an
// error for a kind mismatch, and an occurrence it treats as absent (null or a
// contract-breaking kind) leaves the receiver, including a value decoded from an
// earlier duplicate member, untouched. A later readable occurrence replaces an
// earlier one, as for any encoding/json field.

// ErrorTextMember decodes an optional string member of a provider error object.
// Any other JSON kind, including null, is recorded as absent: provider error
// metadata is unvalidated, and one malformed optional member must not discard
// the rest of the error object. Error decoding and the relay's upstream error
// envelope decoder (relay/controller) both use it, so string members are read
// alike on every upstream error path.
type ErrorTextMember struct {
	value string
	ok    bool
}

// Value returns the decoded string.
//
// Returns:
//   - string: the member's text, or "" when the member was absent or not a JSON string.
func (m ErrorTextMember) Value() string {
	return m.value
}

// Present reports whether the member was a JSON string, including the empty string.
//
// Returns:
//   - bool: true when Value holds a decoded JSON string.
func (m ErrorTextMember) Present() bool {
	return m.ok
}

// UnmarshalJSON implements json.Unmarshaler for ErrorTextMember. It updates the
// receiver only after decoding a JSON string; null and every other kind are
// absent and leave the receiver untouched.
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - error: a wrapped decode failure for a malformed JSON string; other kinds are absent.
func (m *ErrorTextMember) UnmarshalJSON(data []byte) error {
	if ErrorJSONKind(data) != '"' {
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.Wrap(err, "decode error text member")
	}
	m.value, m.ok = value, true
	return nil
}

// errorCodeMember decodes the code member of Error, which keeps the `any`
// decoding encoding/json applies (string, float64, bool, map or slice). Null
// and a number outside the float64 range are absent and leave the receiver
// untouched. The relay's client-facing envelope decoder deliberately does not
// reuse it: that decoder accepts only string and number codes.
type errorCodeMember struct {
	value any
	ok    bool
}

// UnmarshalJSON implements json.Unmarshaler for errorCodeMember.
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - error: a wrapped decode failure other than an unrepresentable number.
func (m *errorCodeMember) UnmarshalJSON(data []byte) error {
	if ErrorJSONKind(data) == 'n' {
		return nil
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			// Optional metadata that cannot be represented is absent; failing
			// here would discard the readable members of the error object.
			return nil
		}
		return errors.Wrap(err, "decode error code member")
	}
	m.value, m.ok = value, true
	return nil
}

// errorWire is the tolerant decoding form of Error. Its tags keep encoding/json's
// case-insensitive key matching.
type errorWire struct {
	Message ErrorTextMember `json:"message"`
	Type    ErrorTextMember `json:"type"`
	Param   ErrorTextMember `json:"param"`
	Code    errorCodeMember `json:"code"`
}

// ErrorJSONKind returns the first significant byte of a raw JSON value, which
// identifies its kind ('{', '[', '"', 't', 'f', 'n', '-' or a digit).
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - byte: the kind marker, or 0 for blank input.
func ErrorJSONKind(data []byte) byte {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 {
		return 0
	}
	return trimmed[0]
}

// ErrorRootTypeError reports a JSON root that cannot hold an error object, in
// the same *json.UnmarshalTypeError form the default struct decoder produces.
// The type error is wrapped with a stack: errors.As still finds the
// *json.UnmarshalTypeError, and Error() returns exactly the type error's text.
//
// Parameters:
//   - kind: the root's kind marker from ErrorJSONKind.
//   - target: the Go type being decoded.
//
// Returns:
//   - error: a stack-carrying error wrapping the *json.UnmarshalTypeError.
func ErrorRootTypeError(kind byte, target reflect.Type) error {
	value := "number"
	switch kind {
	case '[':
		value = "array"
	case '"':
		value = "string"
	case 't', 'f':
		value = "bool"
	}
	return errors.WithStack(&json.UnmarshalTypeError{Value: value, Type: target, Offset: 1})
}

// UnmarshalJSON decodes an error object received over the wire and marks it
// FromUpstream: JSON bytes never originate in this process, so a decoded
// "one_api_error" describes the provider (for example another one-api behind a
// channel), never this gateway. A repository-wide sweep (2026-11-19) found no
// path that JSON-round-trips a locally produced error into the relay
// classifiers; such a path would have to rebuild the Error instead.
//
// The object is decoded tolerantly: message, type and param members that are
// not JSON strings are absent, and code keeps its `any` decoding except that
// null and an out-of-range number are absent. Absent members, including null
// or contract-breaking duplicates, leave existing values untouched. A null root
// is a no-op, a non-object root fails with *json.UnmarshalTypeError, and
// server-only fields (MessageContainsRawBody, RawError) are never written.
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - error: a type mismatch for a non-object root, or a wrapped decode failure.
func (e *Error) UnmarshalJSON(data []byte) error {
	switch kind := ErrorJSONKind(data); kind {
	case 'n':
		return nil
	case '{':
	default:
		return ErrorRootTypeError(kind, reflect.TypeFor[Error]())
	}

	var wire errorWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode error object")
	}
	if wire.Message.ok {
		e.Message = wire.Message.value
	}
	if wire.Type.ok {
		e.Type = ErrorType(wire.Type.value)
	}
	if wire.Param.ok {
		e.Param = wire.Param.value
	}
	if wire.Code.ok {
		e.Code = wire.Code.value
	}
	e.FromUpstream = true
	return nil
}

// UnmarshalJSON decodes the flat JSON form of ErrorWithStatusCode: the embedded
// Error members plus status_code. It must exist because Error.UnmarshalJSON
// would otherwise be promoted through the embedding and silently drop
// status_code. An absent or null status_code keeps the existing value.
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - error: a type mismatch for a non-object root or status_code, or a wrapped decode failure.
func (e *ErrorWithStatusCode) UnmarshalJSON(data []byte) error {
	switch kind := ErrorJSONKind(data); kind {
	case 'n':
		return nil
	case '{':
	default:
		return ErrorRootTypeError(kind, reflect.TypeFor[ErrorWithStatusCode]())
	}

	if err := e.Error.UnmarshalJSON(data); err != nil {
		return errors.Wrap(err, "decode embedded error")
	}
	status := struct {
		StatusCode int `json:"status_code"`
	}{StatusCode: e.StatusCode}
	if err := json.Unmarshal(data, &status); err != nil {
		return errors.Wrap(err, "decode error status_code")
	}
	e.StatusCode = status.StatusCode
	return nil
}
