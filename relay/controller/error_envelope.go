package controller

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// This file holds the wire decoding of upstream error bodies. HTTP handling
// (bounded reads, status and the generic fallback messages) lives in error.go
// and reaches this decoder only through decodeUpstreamErrorEnvelope.
//
// String members reuse model.ErrorTextMember, model.ErrorJSONKind and
// model.ErrorRootTypeError, so they decode exactly as model.Error decodes them.
// The code member deliberately does not reuse model.Error's `any` decoding:
// this envelope feeds the client-facing error, so it keeps only string and
// number codes (see upstreamErrorCode).
//
// Every decoder below follows the json.Unmarshaler convention: encoding/json
// matches keys case-insensitively and calls UnmarshalJSON once per occurrence of
// a member (null included) on the same receiver, so an occurrence a decoder
// treats as absent leaves the receiver untouched, and the receiver changes only
// after an accepted kind decoded successfully. A readable value decoded from an
// earlier duplicate member therefore survives a later null or unusable one.

// GeneralErrorResponse is the normalized view of an upstream error body. It
// covers the OpenAI nested envelope ({"error":{...}}), flat OpenAI-compatible
// envelopes ({"message","type","code","param"}), the legacy message keys several
// providers use, and the FastAPI {"detail":"..."} convention. Decode it with
// json.Unmarshal: UnmarshalJSON is tolerant of optional fields whose JSON kind
// differs from this contract.
type GeneralErrorResponse struct {
	Error model.Error `json:"error"`
	// ErrorText holds a bare string "error" member, e.g. {"error":"model not found"}.
	ErrorText string          `json:"-"`
	Message   string          `json:"message"`
	Type      model.ErrorType `json:"type"`
	Code      any             `json:"code"`
	// Param is nil when the provider sent no usable string param.
	Param    *string `json:"param"`
	Msg      string  `json:"msg"`
	Err      string  `json:"err"`
	ErrorMsg string  `json:"error_msg"`
	Header   struct {
		Message string `json:"message"`
	} `json:"header"`
	Response struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	// Detail holds a string FastAPI-style "detail" member. A non-string detail,
	// such as a FastAPI validation error list, is absent.
	Detail string `json:"detail"`
}

// decodeUpstreamErrorEnvelope decodes an upstream error body into the
// normalized envelope. It performs no HTTP handling: callers bound, read and
// classify the body (empty, oversized, unreadable) before calling it.
//
// Parameters:
//   - body: the complete, bounded upstream error body.
//
// Returns:
//   - GeneralErrorResponse: the decoded envelope; the zero value on failure.
//   - error: a decode failure, including a *json.UnmarshalTypeError for a
//     non-object root, so the caller falls back to a generic message.
func decodeUpstreamErrorEnvelope(body []byte) (GeneralErrorResponse, error) {
	var envelope GeneralErrorResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return GeneralErrorResponse{}, errors.Wrap(err, "decode upstream error envelope")
	}
	return envelope, nil
}

// upstreamErrorCode decodes the code member, which providers send as a string or
// a number (decoded as float64, as encoding/json decodes numbers into any). Both
// nested and flat envelopes use it, so they treat codes alike. Other kinds,
// including null, are absent: an object or bool code can carry provider
// internals and must not reach the client. A nil value means no usable code was
// decoded.
type upstreamErrorCode struct {
	value any
}

// UnmarshalJSON implements json.Unmarshaler for upstreamErrorCode.
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - error: a wrapped decode failure for a malformed JSON string. A number
//     outside the float64 range is optional metadata that cannot be represented,
//     so it is absent rather than failing the whole envelope. An absent
//     occurrence leaves the receiver untouched.
func (c *upstreamErrorCode) UnmarshalJSON(data []byte) error {
	switch kind := model.ErrorJSONKind(data); {
	case kind == '"':
		var code string
		if err := json.Unmarshal(data, &code); err != nil {
			return errors.Wrap(err, "decode upstream error code")
		}
		c.value = code
	case kind == '-' || (kind >= '0' && kind <= '9'):
		code, err := strconv.ParseFloat(string(bytes.TrimSpace(data)), 64)
		if err == nil {
			c.value = code
		}
	}
	return nil
}

// upstreamErrorObject is the tolerant wire form of an OpenAI-style error object.
type upstreamErrorObject struct {
	Message model.ErrorTextMember `json:"message"`
	Type    model.ErrorTextMember `json:"type"`
	Param   model.ErrorTextMember `json:"param"`
	Code    upstreamErrorCode     `json:"code"`
}

// upstreamNestedError decodes the "error" member, which is an object for OpenAI
// envelopes and a bare string for several providers. Other kinds, including
// null, are absent. Repeated object occurrences merge member by member, as
// encoding/json merges repeated struct members.
type upstreamNestedError struct {
	object upstreamErrorObject
	text   model.ErrorTextMember
}

// UnmarshalJSON implements json.Unmarshaler for upstreamNestedError.
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - error: a wrapped decode failure for a malformed object or string; the
//     receiver is then left untouched.
func (e *upstreamNestedError) UnmarshalJSON(data []byte) error {
	switch model.ErrorJSONKind(data) {
	case '{':
		object := e.object
		if err := json.Unmarshal(data, &object); err != nil {
			return errors.Wrap(err, "decode upstream error object")
		}
		e.object = object
	case '"':
		text := e.text
		if err := json.Unmarshal(data, &text); err != nil {
			return errors.Wrap(err, "decode upstream error string")
		}
		e.text = text
	}
	return nil
}

// upstreamMessageObject decodes {"message": "..."} and ignores any other kind.
type upstreamMessageObject struct {
	Message model.ErrorTextMember
}

// UnmarshalJSON implements json.Unmarshaler for upstreamMessageObject. A
// non-object occurrence is absent; an object occurrence without a usable
// message keeps the message decoded from an earlier duplicate member.
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - error: a wrapped decode failure for a malformed object; the receiver is
//     then left untouched.
func (o *upstreamMessageObject) UnmarshalJSON(data []byte) error {
	if model.ErrorJSONKind(data) != '{' {
		return nil
	}
	wire := struct {
		Message model.ErrorTextMember `json:"message"`
	}{Message: o.Message}
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode upstream message object")
	}
	o.Message = wire.Message
	return nil
}

// upstreamResponseObject decodes {"error": {"message": "..."}} and ignores any other kind.
type upstreamResponseObject struct {
	Error upstreamMessageObject
}

// UnmarshalJSON implements json.Unmarshaler for upstreamResponseObject. A
// non-object occurrence is absent; an object occurrence without a usable error
// message keeps the message decoded from an earlier duplicate member.
//
// Parameters:
//   - data: one raw JSON value.
//
// Returns:
//   - error: a wrapped decode failure for a malformed object; the receiver is
//     then left untouched.
func (o *upstreamResponseObject) UnmarshalJSON(data []byte) error {
	if model.ErrorJSONKind(data) != '{' {
		return nil
	}
	wire := struct {
		Error upstreamMessageObject `json:"error"`
	}{Error: o.Error}
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode upstream response object")
	}
	o.Error = wire.Error
	return nil
}

// generalErrorResponseWire is the tolerant decoding form of GeneralErrorResponse.
// It keeps encoding/json's case-insensitive key matching so legacy providers that
// send keys such as "Message" still parse.
type generalErrorResponseWire struct {
	Error    upstreamNestedError    `json:"error"`
	Message  model.ErrorTextMember  `json:"message"`
	Type     model.ErrorTextMember  `json:"type"`
	Code     upstreamErrorCode      `json:"code"`
	Param    model.ErrorTextMember  `json:"param"`
	Msg      model.ErrorTextMember  `json:"msg"`
	Err      model.ErrorTextMember  `json:"err"`
	ErrorMsg model.ErrorTextMember  `json:"error_msg"`
	Header   upstreamMessageObject  `json:"header"`
	Response upstreamResponseObject `json:"response"`
	Detail   model.ErrorTextMember  `json:"detail"`
}

// UnmarshalJSON decodes an upstream error body tolerantly. The root must be a
// JSON object (or null); any other root fails exactly as the strict decoder did,
// so non-JSON and non-object bodies take the generic unrecognized-body message.
// Inside the object, a member whose JSON kind does not match its contract is
// absent. As with encoding/json struct decoding, an absent member leaves the
// receiver's field untouched, so repeated occurrences merge.
//
// Parameters:
//   - data: the raw upstream response body.
//
// Returns:
//   - error: a stack-carrying *json.UnmarshalTypeError for a non-object root, or
//     a wrapped decode failure; the receiver is then left untouched.
func (e *GeneralErrorResponse) UnmarshalJSON(data []byte) error {
	switch kind := model.ErrorJSONKind(data); kind {
	case 'n':
		return nil
	case '{':
	default:
		return model.ErrorRootTypeError(kind, reflect.TypeFor[GeneralErrorResponse]())
	}

	var wire generalErrorResponseWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode upstream error envelope")
	}
	wire.applyTo(e)
	return nil
}

// applyTo copies every member that was present in the decoded wire form onto e
// and leaves the other fields of e untouched.
//
// Parameters:
//   - e: the envelope to update.
func (w *generalErrorResponseWire) applyTo(e *GeneralErrorResponse) {
	// The nested error is rebuilt from wire bytes rather than decoded by
	// (*model.Error).UnmarshalJSON, so it carries the same provenance mark
	// explicitly: an envelope never originates in this process.
	nested := w.Error.object
	e.Error.FromUpstream = true
	setErrorText(&e.Error.Message, nested.Message)
	if nested.Type.Present() {
		e.Error.Type = model.ErrorType(nested.Type.Value())
	}
	setErrorText(&e.Error.Param, nested.Param)
	if nested.Code.value != nil {
		e.Error.Code = nested.Code.value
	}

	setErrorText(&e.ErrorText, w.Error.text)
	setErrorText(&e.Message, w.Message)
	if w.Type.Present() {
		e.Type = model.ErrorType(w.Type.Value())
	}
	if w.Code.value != nil {
		e.Code = w.Code.value
	}
	if w.Param.Present() {
		param := w.Param.Value()
		e.Param = &param
	}
	setErrorText(&e.Msg, w.Msg)
	setErrorText(&e.Err, w.Err)
	setErrorText(&e.ErrorMsg, w.ErrorMsg)
	setErrorText(&e.Header.Message, w.Header.Message)
	setErrorText(&e.Response.Error.Message, w.Response.Error.Message)
	setErrorText(&e.Detail, w.Detail)
}

// setErrorText stores member's string in dst when the member was present and
// leaves dst untouched otherwise.
//
// Parameters:
//   - dst: the field to update.
//   - member: the decoded optional string member.
func setErrorText(dst *string, member model.ErrorTextMember) {
	if member.Present() {
		*dst = member.Value()
	}
}

// ToMessage returns the first non-empty human-readable message in the envelope,
// preferring the nested OpenAI error, then a bare string error, then flat and
// legacy provider keys, and finally a string FastAPI "detail".
//
// Returns:
//   - string: the selected message, or "" when the body carries no message.
func (e GeneralErrorResponse) ToMessage() string {
	for _, message := range []string{
		e.Error.Message, e.ErrorText, e.Message, e.Msg, e.Err, e.ErrorMsg,
		e.Header.Message, e.Response.Error.Message, e.Detail,
	} {
		if message != "" {
			return message
		}
	}
	return ""
}
