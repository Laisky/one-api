package model

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestErrorUnmarshalMarksWireProvenance requires every Error decoded from JSON
// to be attributed to the provider that produced the bytes: a chained one-api
// behind a channel answers with "one_api_error", which must never be treated as
// this gateway's own error by IsOneAPIError.
func TestErrorUnmarshalMarksWireProvenance(t *testing.T) {
	t.Parallel()

	var decoded Error
	require.NoError(t, json.Unmarshal([]byte(`{"message":"quota exhausted","type":"one_api_error","param":"p","code":"insufficient_user_quota"}`), &decoded))
	require.Equal(t, Error{Message: "quota exhausted", Type: ErrorTypeOneAPI, Param: "p", Code: "insufficient_user_quota", FromUpstream: true}, decoded)
	require.False(t, decoded.IsOneAPIError())

	// Provider envelopes embed Error in their own response structs.
	var envelope struct {
		Error  Error  `json:"error"`
		Nested *Error `json:"nested"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"error":{"message":"m","type":"one_api_error"},"nested":{"type":"one_api_error"}}`), &envelope))
	require.False(t, envelope.Error.IsOneAPIError())
	require.NotNil(t, envelope.Nested)
	require.False(t, envelope.Nested.IsOneAPIError())

	// A locally constructed value with the same Type remains one-api's own error.
	require.True(t, Error{Type: ErrorTypeOneAPI}.IsOneAPIError())
}

// TestErrorUnmarshalTolerantMembers requires message, type and param members
// whose JSON kind breaks the string contract to be absent instead of failing
// the whole decode, while code keeps its `any` decoding (an out-of-range number
// is absent) and keys keep encoding/json's case-insensitive matching.
func TestErrorUnmarshalTolerantMembers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, body string
		want       Error
	}{
		{"object_param", `{"message":"bad","type":"invalid_request_error","param":{"field":"x"},"code":"c"}`,
			Error{Message: "bad", Type: ErrorTypeInvalidRequest, Code: "c", FromUpstream: true}},
		{"object_message", `{"message":{"detail":"x"},"type":"rate_limit","code":"quota_exceeded"}`,
			Error{Type: "rate_limit", Code: "quota_exceeded", FromUpstream: true}},
		{"numeric_message", `{"message":42,"type":"one_api_error","code":7}`,
			Error{Type: ErrorTypeOneAPI, Code: float64(7), FromUpstream: true}},
		{"array_type_bool_param", `{"message":"m","type":["x"],"param":true}`,
			Error{Message: "m", FromUpstream: true}},
		{"null_members", `{"message":null,"type":null,"param":null,"code":null}`,
			Error{FromUpstream: true}},
		{"case_insensitive_keys", `{"Message":"m","TYPE":"one_api_error","Param":"p","Code":"c"}`,
			Error{Message: "m", Type: ErrorTypeOneAPI, Param: "p", Code: "c", FromUpstream: true}},
		{"out_of_range_code", `{"message":"m","code":1e999}`,
			Error{Message: "m", FromUpstream: true}},
		{"object_code", `{"message":"m","code":{"reason":"r"}}`,
			Error{Message: "m", Code: map[string]any{"reason": "r"}, FromUpstream: true}},
		{"bool_code", `{"message":"m","code":false}`,
			Error{Message: "m", Code: false, FromUpstream: true}},
		{"unknown_members", `{"message":"m","status_code":500,"extra":[1,2]}`,
			Error{Message: "m", FromUpstream: true}},
		{"empty_object", `{}`, Error{FromUpstream: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var decoded Error
			require.NoError(t, json.Unmarshal([]byte(tc.body), &decoded))
			require.Equal(t, tc.want, decoded)
			require.False(t, decoded.IsOneAPIError())
		})
	}
}

// TestErrorUnmarshalDuplicateMembers requires a null or contract-breaking
// duplicate member (matched case-insensitively) to leave a readable value from
// an earlier occurrence untouched, while a later readable duplicate wins.
func TestErrorUnmarshalDuplicateMembers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, body string
		want       Error
	}{
		{"message_then_null", `{"message":"quota exceeded","Message":null}`, Error{Message: "quota exceeded", FromUpstream: true}},
		{"message_then_object", `{"message":"quota exceeded","MESSAGE":{"x":1}}`, Error{Message: "quota exceeded", FromUpstream: true}},
		{"type_then_number", `{"type":"rate_limit_error","Type":5}`, Error{Type: ErrorTypeRateLimit, FromUpstream: true}},
		{"param_then_array", `{"param":"model","Param":["x"]}`, Error{Param: "model", FromUpstream: true}},
		{"code_then_null", `{"code":"AccountQuotaExceeded","Code":null}`, Error{Code: "AccountQuotaExceeded", FromUpstream: true}},
		{"code_then_out_of_range", `{"code":"AccountQuotaExceeded","Code":1e400}`, Error{Code: "AccountQuotaExceeded", FromUpstream: true}},
		{"later_string_wins", `{"message":"first","Message":"second"}`, Error{Message: "second", FromUpstream: true}},
		{"later_code_wins", `{"code":"first","Code":2}`, Error{Code: float64(2), FromUpstream: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var decoded Error
			require.NoError(t, json.Unmarshal([]byte(tc.body), &decoded))
			require.Equal(t, tc.want, decoded)
		})
	}
}

// TestErrorUnmarshalKeepsDecoderSemantics requires the encoding/json behaviors
// callers rely on: a null root is a no-op, a non-object root still fails with
// *json.UnmarshalTypeError, absent and null members leave existing values
// untouched, and server-only fields are never written by JSON.
func TestErrorUnmarshalKeepsDecoderSemantics(t *testing.T) {
	t.Parallel()

	var decoded Error
	require.NoError(t, json.Unmarshal([]byte(`null`), &decoded))
	require.Equal(t, Error{}, decoded)

	var wrapper struct {
		Error Error  `json:"error"`
		Ptr   *Error `json:"ptr"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"error":null,"ptr":null}`), &wrapper))
	require.Equal(t, Error{}, wrapper.Error)
	require.Nil(t, wrapper.Ptr)

	for _, body := range []string{`"text"`, `[1]`, `12`, `true`} {
		var root Error
		err := json.Unmarshal([]byte(body), &root)
		var typeErr *json.UnmarshalTypeError
		require.True(t, errors.As(err, &typeErr), body)
		require.False(t, root.FromUpstream, body)
	}
	var malformed Error
	require.Error(t, json.Unmarshal([]byte(`{"message":"m"`), &malformed))

	raw := errors.New("diagnostic")
	existing := Error{Message: "keep", Param: "keep-param", Code: "old", MessageContainsRawBody: true, RawError: raw}
	require.NoError(t, json.Unmarshal([]byte(`{"message":null,"type":"server_error","code":null}`), &existing))
	require.Equal(t, Error{Message: "keep", Type: ErrorTypeServer, Param: "keep-param", Code: "old",
		MessageContainsRawBody: true, FromUpstream: true, RawError: raw}, existing)

	var injected Error
	require.NoError(t, json.Unmarshal([]byte(`{"type":"one_api_error","FromUpstream":false,"MessageContainsRawBody":true,"RawError":"x"}`), &injected))
	require.Equal(t, Error{Type: ErrorTypeOneAPI, FromUpstream: true}, injected)
	require.False(t, injected.IsOneAPIError())
}

// TestErrorWithStatusCodeUnmarshalKeepsStatus requires ErrorWithStatusCode not
// to inherit a promoted Error.UnmarshalJSON that would silently drop
// status_code, and its embedded error to be marked as provider-sourced.
func TestErrorWithStatusCodeUnmarshalKeepsStatus(t *testing.T) {
	t.Parallel()

	var decoded ErrorWithStatusCode
	require.NoError(t, json.Unmarshal([]byte(`{"message":"m","type":"one_api_error","param":{"x":1},"code":"insufficient_user_quota","status_code":403}`), &decoded))
	require.Equal(t, ErrorWithStatusCode{
		Error:      Error{Message: "m", Type: ErrorTypeOneAPI, Code: "insufficient_user_quota", FromUpstream: true},
		StatusCode: http.StatusForbidden,
	}, decoded)
	require.False(t, decoded.IsOneAPIError())

	existing := ErrorWithStatusCode{StatusCode: http.StatusBadGateway}
	require.NoError(t, json.Unmarshal([]byte(`{"message":"m","status_code":null}`), &existing))
	require.Equal(t, http.StatusBadGateway, existing.StatusCode, "a null status keeps the existing value")
	require.Equal(t, "m", existing.Message)

	var nullRoot ErrorWithStatusCode
	require.NoError(t, json.Unmarshal([]byte(`null`), &nullRoot))
	require.Equal(t, ErrorWithStatusCode{}, nullRoot)

	var invalid ErrorWithStatusCode
	require.Error(t, json.Unmarshal([]byte(`[]`), &invalid))
	require.Error(t, json.Unmarshal([]byte(`{"status_code":"500"}`), &invalid))
}

// TestErrorMarshalUnchanged pins the client-visible JSON of Error and
// ErrorWithStatusCode: server-only provenance and diagnostics never appear, and
// a decode of that JSON restores every public member.
func TestErrorMarshalUnchanged(t *testing.T) {
	t.Parallel()

	value := Error{Message: "m", Type: ErrorTypeOneAPI, Param: "p", Code: "c",
		MessageContainsRawBody: true, FromUpstream: true, RawError: errors.New("raw")}
	payload, err := json.Marshal(value)
	require.NoError(t, err)
	require.Equal(t, `{"message":"m","type":"one_api_error","param":"p","code":"c"}`, string(payload))

	withStatus, err := json.Marshal(ErrorWithStatusCode{Error: value, StatusCode: http.StatusTooManyRequests})
	require.NoError(t, err)
	require.Equal(t, `{"message":"m","type":"one_api_error","param":"p","code":"c","status_code":429}`, string(withStatus))

	pointer, err := json.Marshal(&ErrorWithStatusCode{Error: value, StatusCode: http.StatusTooManyRequests})
	require.NoError(t, err)
	require.Equal(t, string(withStatus), string(pointer))

	var roundTrip ErrorWithStatusCode
	require.NoError(t, json.Unmarshal(withStatus, &roundTrip))
	require.Equal(t, ErrorWithStatusCode{
		Error:      Error{Message: "m", Type: ErrorTypeOneAPI, Param: "p", Code: "c", FromUpstream: true},
		StatusCode: http.StatusTooManyRequests,
	}, roundTrip)
}
