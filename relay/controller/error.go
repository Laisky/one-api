package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
)

// GeneralErrorResponse is the normalized view of an upstream error body. It
// covers the OpenAI nested envelope ({"error":{...}}), flat OpenAI-compatible
// envelopes ({"message","type","code","param"}), and the legacy message keys
// several providers use. Decode it with json.Unmarshal: UnmarshalJSON is tolerant
// of optional fields whose JSON kind differs from this contract.
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
}

// upstreamErrorText decodes a JSON string and treats every other JSON kind,
// including null, as absent. Provider error metadata is optional and unvalidated;
// an object where a string belongs must cost only that field, never the whole
// body, because the whole-body fallback echoes the raw upstream response.
type upstreamErrorText struct {
	value string
	ok    bool
}

// UnmarshalJSON implements json.Unmarshaler for upstreamErrorText.
//
// Parameters:
//   - data: one raw JSON value.
//
// Return values:
//   - error: a wrapped decode failure for a malformed JSON string; other kinds are ignored.
func (t *upstreamErrorText) UnmarshalJSON(data []byte) error {
	*t = upstreamErrorText{}
	if jsonValueKind(data) != '"' {
		return nil
	}
	if err := json.Unmarshal(data, &t.value); err != nil {
		return errors.Wrap(err, "decode upstream error text")
	}
	t.ok = true
	return nil
}

// upstreamErrorCode decodes the code member, which providers send as a string or
// a number (decoded as float64, as encoding/json decodes numbers into any). Both
// nested and flat envelopes use it, so they treat codes alike. Other kinds,
// including null, are absent.
type upstreamErrorCode struct {
	value any
}

// UnmarshalJSON implements json.Unmarshaler for upstreamErrorCode.
//
// Parameters:
//   - data: one raw JSON value.
//
// Return values:
//   - error: a wrapped decode failure for a malformed JSON string. A number
//     outside the float64 range is optional metadata that cannot be represented,
//     so it is treated as absent rather than failing the whole envelope.
func (c *upstreamErrorCode) UnmarshalJSON(data []byte) error {
	*c = upstreamErrorCode{}
	switch kind := jsonValueKind(data); {
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
	Message upstreamErrorText `json:"message"`
	Type    upstreamErrorText `json:"type"`
	Param   upstreamErrorText `json:"param"`
	Code    upstreamErrorCode `json:"code"`
}

// upstreamNestedError decodes the "error" member, which is an object for OpenAI
// envelopes and a bare string for several providers. Other kinds are absent.
type upstreamNestedError struct {
	object upstreamErrorObject
	text   upstreamErrorText
}

// UnmarshalJSON implements json.Unmarshaler for upstreamNestedError.
//
// Parameters:
//   - data: one raw JSON value.
//
// Return values:
//   - error: a wrapped decode failure for a malformed object or string.
func (e *upstreamNestedError) UnmarshalJSON(data []byte) error {
	*e = upstreamNestedError{}
	switch jsonValueKind(data) {
	case '{':
		if err := json.Unmarshal(data, &e.object); err != nil {
			return errors.Wrap(err, "decode upstream error object")
		}
	case '"':
		if err := json.Unmarshal(data, &e.text); err != nil {
			return errors.Wrap(err, "decode upstream error string")
		}
	}
	return nil
}

// upstreamMessageObject decodes {"message": "..."} and ignores any other kind.
type upstreamMessageObject struct {
	Message upstreamErrorText
}

// UnmarshalJSON implements json.Unmarshaler for upstreamMessageObject.
//
// Parameters:
//   - data: one raw JSON value.
//
// Return values:
//   - error: a wrapped decode failure for a malformed object.
func (o *upstreamMessageObject) UnmarshalJSON(data []byte) error {
	*o = upstreamMessageObject{}
	if jsonValueKind(data) != '{' {
		return nil
	}
	var wire struct {
		Message upstreamErrorText `json:"message"`
	}
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

// UnmarshalJSON implements json.Unmarshaler for upstreamResponseObject.
//
// Parameters:
//   - data: one raw JSON value.
//
// Return values:
//   - error: a wrapped decode failure for a malformed object.
func (o *upstreamResponseObject) UnmarshalJSON(data []byte) error {
	*o = upstreamResponseObject{}
	if jsonValueKind(data) != '{' {
		return nil
	}
	var wire struct {
		Error upstreamMessageObject `json:"error"`
	}
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
	Message  upstreamErrorText      `json:"message"`
	Type     upstreamErrorText      `json:"type"`
	Code     upstreamErrorCode      `json:"code"`
	Param    upstreamErrorText      `json:"param"`
	Msg      upstreamErrorText      `json:"msg"`
	Err      upstreamErrorText      `json:"err"`
	ErrorMsg upstreamErrorText      `json:"error_msg"`
	Header   upstreamMessageObject  `json:"header"`
	Response upstreamResponseObject `json:"response"`
}

// jsonValueKind returns the first significant byte of a raw JSON value, which
// identifies its kind ('{', '[', '"', 't', 'f', 'n', '-' or a digit), or 0.
func jsonValueKind(data []byte) byte {
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) == 0 {
		return 0
	}
	return trimmed[0]
}

// UnmarshalJSON decodes an upstream error body tolerantly. The root must be a
// JSON object (or null); any other root fails exactly as the strict decoder did,
// so non-JSON and non-object bodies keep the raw-body fallback. Inside the object,
// a member whose JSON kind does not match its contract is treated as absent.
//
// Parameters:
//   - data: the raw upstream response body.
//
// Return values:
//   - error: a *json.UnmarshalTypeError for a non-object root, or a wrapped decode failure.
func (e *GeneralErrorResponse) UnmarshalJSON(data []byte) error {
	switch kind := jsonValueKind(data); kind {
	case 'n':
		return nil
	case '{':
	default:
		value := map[byte]string{'[': "array", '"': "string", 't': "bool", 'f': "bool"}[kind]
		if value == "" {
			value = "number"
		}
		return &json.UnmarshalTypeError{Value: value, Type: reflect.TypeFor[GeneralErrorResponse](), Offset: 1}
	}

	var wire generalErrorResponseWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return errors.Wrap(err, "decode upstream error envelope")
	}
	nested := wire.Error.object
	*e = GeneralErrorResponse{
		Error: model.Error{
			Message: nested.Message.value,
			Type:    model.ErrorType(nested.Type.value),
			Param:   nested.Param.value,
			Code:    nested.Code.value,
		},
		ErrorText: wire.Error.text.value,
		Message:   wire.Message.value,
		Type:      model.ErrorType(wire.Type.value),
		Code:      wire.Code.value,
		Msg:       wire.Msg.value,
		Err:       wire.Err.value,
		ErrorMsg:  wire.ErrorMsg.value,
	}
	if wire.Param.ok {
		e.Param = &wire.Param.value
	}
	e.Header.Message = wire.Header.Message.value
	e.Response.Error.Message = wire.Response.Error.Message.value
	return nil
}

// ToMessage returns the first non-empty human-readable message in the envelope,
// preferring the nested OpenAI error, then a bare string error, then flat and
// legacy provider keys. It returns "" when the body carries no message.
func (e GeneralErrorResponse) ToMessage() string {
	if e.Error.Message != "" {
		return e.Error.Message
	}
	if e.ErrorText != "" {
		return e.ErrorText
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != "" {
		return e.Err
	}
	if e.ErrorMsg != "" {
		return e.ErrorMsg
	}
	if e.Header.Message != "" {
		return e.Header.Message
	}
	if e.Response.Error.Message != "" {
		return e.Response.Error.Message
	}
	return ""
}

// RelayErrorHandler parses upstream error responses into our unified error model.
// For request-scoped logging, prefer RelayErrorHandlerWithContext.
func RelayErrorHandler(resp *http.Response) (ErrorWithStatusCode *model.ErrorWithStatusCode) {
	if resp == nil {
		return &model.ErrorWithStatusCode{
			StatusCode: 500,
			Error: model.Error{
				Message:  "resp is nil",
				Type:     model.ErrorTypeUpstream,
				Code:     "bad_response",
				RawError: errors.New("resp is nil"),
			},
		}
	}

	// Everything below describes the provider's response, so the result is marked
	// FromUpstream: a provider-supplied "one_api_error" type must never be trusted
	// as this gateway's own error.
	ErrorWithStatusCode = &model.ErrorWithStatusCode{
		StatusCode: resp.StatusCode,
		Error: model.Error{
			Message:      "",
			Type:         model.ErrorTypeUpstream,
			Code:         "bad_response_status_code",
			Param:        strconv.Itoa(resp.StatusCode),
			FromUpstream: true,
			RawError:     errors.Errorf("bad response %d", resp.StatusCode),
		},
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		ErrorWithStatusCode.Error.Message = fmt.Sprintf("failed to read response body: %+v\n\n", err) + ErrorWithStatusCode.Error.Message
		ErrorWithStatusCode.Error.RawError = err
		return
	}

	// Intentionally avoid global logger here; use context variant where possible.
	err = resp.Body.Close()
	if err != nil {
		ErrorWithStatusCode.Error.Message = fmt.Sprintf("failed to close response body: %+v\n\n", err) + ErrorWithStatusCode.Error.Message
		ErrorWithStatusCode.Error.RawError = err
		return
	}

	var errResponse GeneralErrorResponse
	err = json.Unmarshal(responseBody, &errResponse)
	if err != nil {
		ErrorWithStatusCode.Error.Message = fmt.Sprintf("failed to unmarshal response body: %+v\n\n", err) + ErrorWithStatusCode.Error.Message + string(responseBody)
		ErrorWithStatusCode.Error.MessageContainsRawBody = true
		ErrorWithStatusCode.Error.RawError = err
		return
	}

	if errResponse.Error.Message != "" {
		// OpenAI format error, so we override the default one
		ErrorWithStatusCode.Error = errResponse.Error
		ErrorWithStatusCode.Error.FromUpstream = true
		if ErrorWithStatusCode.Error.RawError == nil && ErrorWithStatusCode.Error.Message != "" {
			ErrorWithStatusCode.Error.RawError = errors.New(ErrorWithStatusCode.Error.Message)
		}
	} else {
		ErrorWithStatusCode.Error.Message = errResponse.ToMessage()
		// Flat OpenAI-compatible envelopes carry the same attribution as nested
		// errors. Keep defaults only for fields the provider did not supply.
		if errResponse.Type != "" {
			ErrorWithStatusCode.Error.Type = errResponse.Type
		}
		if errResponse.Code != nil {
			ErrorWithStatusCode.Error.Code = errResponse.Code
		}
		if errResponse.Param != nil {
			ErrorWithStatusCode.Error.Param = *errResponse.Param
		}
		if ErrorWithStatusCode.Error.RawError == nil && ErrorWithStatusCode.Error.Message != "" {
			ErrorWithStatusCode.Error.RawError = errors.New(ErrorWithStatusCode.Error.Message)
		}
	}

	if ErrorWithStatusCode.Error.Message == "" {
		ErrorWithStatusCode.Error.Message = fmt.Sprintf("bad response [%d] %s", resp.StatusCode, string(responseBody))
		ErrorWithStatusCode.Error.MessageContainsRawBody = true
	}

	return
}

// RelayErrorHandlerWithContext is a context-aware variant that logs using the request-scoped logger.
func RelayErrorHandlerWithContext(c *gin.Context, resp *http.Response) *model.ErrorWithStatusCode {
	if resp == nil {
		return RelayErrorHandler(resp)
	}
	// Read and restore response body for downstream use
	responseBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	logUpstreamResponseFromBytes(gmw.GetLogger(c), resp, responseBody, string(model.ErrorTypeUpstream))
	// Reconstruct a new ReadCloser for any further reads (not commonly needed here)
	resp.Body = io.NopCloser(bytes.NewReader(responseBody))
	return RelayErrorHandler(resp)
}
