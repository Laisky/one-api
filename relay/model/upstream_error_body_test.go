package model

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// TestUnrecognizedUpstreamErrorMessage pins the generic client-visible formats.
func TestUnrecognizedUpstreamErrorMessage(t *testing.T) {
	t.Parallel()

	require.Equal(t, "upstream returned HTTP 502 with an unrecognized error body (content-type text/html, 1234 bytes)",
		UnrecognizedUpstreamErrorMessage(502, "text/html; charset=utf-8", 1234, false))
	require.Equal(t, "upstream returned HTTP 502 with an unrecognized error body (1234 bytes)",
		UnrecognizedUpstreamErrorMessage(502, "", 1234, false))
	require.Equal(t, "upstream returned HTTP 503 with an empty error body",
		UnrecognizedUpstreamErrorMessage(503, "text/plain", 0, false))
	require.Equal(t, "upstream returned HTTP 503 with an empty error body",
		UnrecognizedUpstreamErrorMessage(503, "text/plain", -1, false))
	require.Equal(t, "upstream returned HTTP 502 with an error body larger than the 1048576-byte limit (content-type application/json)",
		UnrecognizedUpstreamErrorMessage(502, "application/json", MaxUpstreamErrorBodyBytes+1, true))
	require.Equal(t, "upstream returned HTTP 502 with an error body larger than the 1048576-byte limit",
		UnrecognizedUpstreamErrorMessage(502, "", MaxUpstreamErrorBodyBytes+1, true))
	require.Equal(t, "upstream returned HTTP 502 with an error body that could not be read",
		UnreadableUpstreamErrorMessage(502))
}

// TestSanitizeUpstreamMediaType requires only a bounded, restricted-charset
// media type to survive; parameters and anything unusual are dropped.
func TestSanitizeUpstreamMediaType(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"text/html":                               "text/html",
		"TEXT/HTML; charset=UTF-8":                "text/html",
		"application/problem+json":                "application/problem+json",
		"application/vnd.api+json; version=2":     "application/vnd.api+json",
		"text/html; account=internal-account-123": "text/html",
		"text/html; broken":                       "text/html",
		"":                                        "",
		"   ":                                     "",
		"text":                                    "",
		"text/html internal-account-123":          "",
		"text/html/extra":                         "",
		"text/h~tml":                              "",
		"text/" + strings.Repeat("x", 120):        "",
		"text/html; p=" + strings.Repeat("x", 300):    "",
		"text/html\r\nX-Injected: internal-account-1": "",
	} {
		require.Equal(t, want, sanitizeUpstreamMediaType(input), input)
	}
}

// TestUpstreamErrorBodyText requires recognized string messages to pass through
// and every other shape, including non-string kinds, to be absent.
func TestUpstreamErrorBodyText(t *testing.T) {
	t.Parallel()

	for body, want := range map[string]string{
		`{"error":"model not found"}`:                         "model not found",
		`{"error":{"message":" quota exceeded ","code":429}}`: "quota exceeded",
		`{"message":"Bad credentials"}`:                       "Bad credentials",
		`{"detail":"The specified version does not exist"}`:   "The specified version does not exist",
		`{"error":{"message":"inner"},"message":"outer"}`:     "inner",
		`{"error":{"code":500},"message":"outer"}`:            "outer",
		`{"error":"","message":"outer"}`:                      "outer",
		`{"message":"","detail":"fallback"}`:                  "fallback",
		`{"message":{"detail":"internal-account-123"}}`:       "",
		`{"detail":[{"msg":"internal-account-123"}]}`:         "",
		`{"Message":"case differs"}`:                          "",
		`<html>internal-account-123</html>`:                   "",
		`["internal-account-123"]`:                            "",
		`"internal-account-123"`:                              "",
		`null`:                                                "",
		``:                                                    "",
	} {
		require.Equal(t, want, UpstreamErrorBodyText([]byte(body)), body)
	}
	oversized := `{"message":"` + strings.Repeat("a", MaxUpstreamErrorBodyBytes) + `"}`
	require.Empty(t, UpstreamErrorBodyText([]byte(oversized)), "an over-limit body may be a truncated prefix and is never parsed")
}

// TestUpstreamErrorBodyMessage requires a parsed message or a generic
// description that never contains body bytes.
func TestUpstreamErrorBodyMessage(t *testing.T) {
	t.Parallel()

	const secret = "internal-account-123"
	require.Equal(t, "quota exceeded", UpstreamErrorBodyMessage(429, "application/json", []byte(`{"error":{"message":"quota exceeded"}}`)))
	require.Equal(t, "upstream returned HTTP 502 with an unrecognized error body (content-type text/html, 33 bytes)",
		UpstreamErrorBodyMessage(502, "text/html", []byte("<html>"+secret+"</html>")))
	require.Equal(t, "upstream returned HTTP 502 with an empty error body", UpstreamErrorBodyMessage(502, "text/html", []byte(" \n")))
	require.Equal(t, "upstream returned HTTP 502 with an error body larger than the 1048576-byte limit",
		UpstreamErrorBodyMessage(502, "", []byte(strings.Repeat(secret, MaxUpstreamErrorBodyBytes/len(secret)+1))))
}

// errorBodyShortReader returns a fixed number of bytes and then a read error,
// standing for a connection reset in the middle of an error body.
type errorBodyShortReader struct {
	served bool
}

// Read implements io.Reader for errorBodyShortReader.
//
// Parameters:
//   - p: the destination buffer.
//
// Returns:
//   - int: 3 on the first call, 0 afterwards.
//   - error: nil on the first call, a reset error afterwards.
func (r *errorBodyShortReader) Read(p []byte) (int, error) {
	if r.served {
		return 0, errors.New("connection reset")
	}
	r.served = true
	return copy(p, "abc"), nil
}

// TestReadBoundedUpstreamBody requires reads to stop one byte past the limit,
// a nil body to read as empty, and a read error to be wrapped while the bytes
// read before it are kept.
func TestReadBoundedUpstreamBody(t *testing.T) {
	t.Parallel()

	data, err := ReadBoundedUpstreamBody(strings.NewReader(strings.Repeat("a", 3*MaxUpstreamErrorBodyBytes)))
	require.NoError(t, err)
	require.Len(t, data, MaxUpstreamErrorBodyBytes+1)

	data, err = ReadBoundedUpstreamBody(strings.NewReader("small"))
	require.NoError(t, err)
	require.Equal(t, "small", string(data))

	data, err = ReadBoundedUpstreamBody(nil)
	require.NoError(t, err)
	require.Empty(t, data)

	data, err = ReadBoundedUpstreamBody(&errorBodyShortReader{})
	require.ErrorContains(t, err, "connection reset")
	require.ErrorContains(t, err, "read bounded upstream body")
	require.Equal(t, "abc", string(data))
	require.NotErrorIs(t, err, io.EOF)
}

// TestErrorTextMemberKinds requires ErrorTextMember to keep only JSON strings
// (including "") and to treat every other kind, including null, as absent
// without touching a previously decoded value.
func TestErrorTextMemberKinds(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, raw, value string
		present          bool
	}{
		{"string", `"capacity"`, "capacity", true},
		{"empty_string", `""`, "", true},
		{"escaped", `"a&b"`, "a&b", true},
		{"null", `null`, "", false},
		{"number", `7`, "", false},
		{"bool", `true`, "", false},
		{"object", `{"account":"internal"}`, "", false},
		{"array", `["internal"]`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var wire struct {
				Member ErrorTextMember `json:"member"`
			}
			require.NoError(t, json.Unmarshal([]byte(`{"member":`+tc.raw+`}`), &wire))
			require.Equal(t, tc.value, wire.Member.Value())
			require.Equal(t, tc.present, wire.Member.Present())

			require.NoError(t, json.Unmarshal([]byte(`{"member":"kept","Member":`+tc.raw+`}`), &wire))
			if !tc.present {
				require.Equal(t, "kept", wire.Member.Value(), "an absent duplicate leaves the earlier value")
				require.True(t, wire.Member.Present())
			}
		})
	}
}

// TestErrorJSONKindAndRootTypeError pins the kind markers and the
// *json.UnmarshalTypeError reported for roots that cannot hold an error object.
func TestErrorJSONKindAndRootTypeError(t *testing.T) {
	t.Parallel()

	require.Equal(t, byte('{'), ErrorJSONKind([]byte(" \t\r\n{}")))
	require.Equal(t, byte('"'), ErrorJSONKind([]byte(`"x"`)))
	require.Equal(t, byte('-'), ErrorJSONKind([]byte("-1")))
	require.Equal(t, byte(0), ErrorJSONKind([]byte(" \n")))
	require.Equal(t, byte(0), ErrorJSONKind(nil))

	target := reflect.TypeFor[Error]()
	for kind, want := range map[byte]string{'[': "array", '"': "string", 't': "bool", 'f': "bool", '1': "number", '-': "number", 0: "number"} {
		var typeErr *json.UnmarshalTypeError
		require.True(t, errors.As(ErrorRootTypeError(kind, target), &typeErr))
		require.Equal(t, want, typeErr.Value, "kind %q", kind)
		require.Equal(t, target, typeErr.Type)
		require.Equal(t, int64(1), typeErr.Offset)
	}
}
