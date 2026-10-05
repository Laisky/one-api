package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// jsonEscapedForTest spells every rune of name as a six-character JSON
// unicode escape (backslash, 'u', four hex digits).
func jsonEscapedForTest(name string) string {
	var b strings.Builder
	for _, r := range name {
		fmt.Fprintf(&b, "%cu%04x", '\\', r)
	}
	return b.String()
}

// foldParityTarget has the typed members whose spellings the parity test probes.
type foldParityTarget struct {
	ExtraBody map[string]any `json:"extra_body"`
	MaxTokens int            `json:"max_tokens"`
	Messages  []any          `json:"messages"`
	Model     string         `json:"model"`
}

// TestFoldJSONKeyMatchesEncodingJSON pins FoldJSONKey to the matching rule of
// the encoding/json decoder in use: a spelling folds like a field name exactly
// when json.Unmarshal stores it into that field. A toolchain change in key
// matching fails here instead of silently reopening a parser differential.
func TestFoldJSONKeyMatchesEncodingJSON(t *testing.T) {
	t.Parallel()
	kelvin, longS := string(rune(0x212A)), string(rune(0x017F))
	for _, tc := range []struct {
		canonical string
		spelling  string
	}{
		{"extra_body", "extra_body"},
		{"extra_body", "Extra_Body"},
		{"extra_body", "EXTRA_BODY"},
		{"extra_body", "extrabody"},
		{"extra_body", "extra-body"},
		{"extra_body", " extra_body"},
		{"extra_body", "extra_body "},
		{"max_tokens", "max_to" + kelvin + "ens"},
		{"max_tokens", "MAX_TOKENS"},
		{"max_tokens", "maxtokens"},
		{"messages", "me" + longS + longS + "ages"},
		{"messages", "MESSAGES"},
		{"model", "Model"},
		{"model", "m0del"},
	} {
		t.Run(tc.canonical+"/"+tc.spelling, func(t *testing.T) {
			t.Parallel()
			value := map[string]string{"extra_body": `{"k":1}`, "max_tokens": `7`, "messages": `[1]`, "model": `"m"`}[tc.canonical]
			key, err := json.Marshal(tc.spelling)
			require.NoError(t, err)
			var target foldParityTarget
			require.NoError(t, json.Unmarshal([]byte(`{`+string(key)+`:`+value+`}`), &target))
			decoded := !reflect.ValueOf(target).IsZero()
			require.Equal(t, decoded, FoldJSONKey(tc.spelling) == FoldJSONKey(tc.canonical))
			require.Equal(t, decoded, JSONKeyMatches(tc.spelling, tc.canonical))
		})
	}
}

// TestScanJSONRootKeysDecodesEscapesAndKeepsDuplicates proves the scan reads
// keys exactly as JSON decoders do, in order, without being confused by
// nested values or by key-like text inside strings.
func TestScanJSONRootKeysDecodesEscapesAndKeepsDuplicates(t *testing.T) {
	t.Parallel()
	body := `{"model":"a","extra` + jsonEscapedForTest("_") + `body":{"x":"}\"{"},"nested":{"extra_body":1,"deep":[{"model":2}]},` +
		`"text":"\"model\":\"b\"","n":null,"flag":true,"num":-1.5e3,"list":[1,"]",{"a":[]}],"model":"c","Model":"d"}`
	keys, err := ScanJSONRootKeys([]byte(body))
	require.NoError(t, err)
	require.Equal(t, []string{"model", "extra_body", "nested", "text", "n", "flag", "num", "list", "model", "Model"}, keys)

	keys, err = ScanJSONRootKeys([]byte(` {} `))
	require.NoError(t, err)
	require.Empty(t, keys)
}

// TestScanJSONRootKeysRejectsNonObjects covers non-object roots and malformed
// or trailing input, which must never be reported as an empty key set.
func TestScanJSONRootKeysRejectsNonObjects(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`null`, `[]`, `"s"`, `1`, ``} {
		_, err := ScanJSONRootKeys([]byte(body))
		require.Error(t, err, body)
	}
	_, err := ScanJSONRootKeys([]byte(`[{"model":"a"}]`))
	require.ErrorIs(t, err, errJSONRootNotObject)
	for _, body := range []string{`{"a":1} {"b":2}`, `{"a":1}x`, `{"a":}`, `{"a":1`, `{"a" 1}`} {
		_, err := ScanJSONRootKeys([]byte(body))
		require.Error(t, err, body)
		require.NotErrorIs(t, err, errJSONRootNotObject, body)
	}
}

// foldEmbedded is promoted into foldFieldsTarget like encoding/json does.
type foldEmbedded struct {
	Promoted string `json:"promoted"`
}

// foldNamedEmbedded is embedded under an explicit tag and is not promoted.
type foldNamedEmbedded struct {
	Hidden string `json:"hidden"`
}

// foldFieldsTarget exercises tag, untagged, ignored, unexported and embedded fields.
type foldFieldsTarget struct {
	foldEmbedded
	*foldNamedEmbedded `json:"named"`
	Tagged             string `json:"tagged,omitempty"`
	Untagged           string
	Ignored            string `json:"-"`
	Dash               string `json:"-,"`
	unexported         string
}

// TestJSONFieldFoldsMirrorsDecoderFieldNames verifies the typed member set
// matches the names encoding/json decodes into the struct.
func TestJSONFieldFoldsMirrorsDecoderFieldNames(t *testing.T) {
	t.Parallel()
	folds := JSONFieldFolds(reflect.TypeOf(&foldFieldsTarget{}))
	names := map[string]bool{}
	for fold, name := range folds {
		require.Equal(t, FoldJSONKey(name), fold)
		names[name] = true
	}
	require.Equal(t, map[string]bool{"promoted": true, "named": true, "tagged": true, "Untagged": true, "-": true}, names)
	require.Nil(t, JSONFieldFolds(reflect.TypeOf(map[string]any{})))
	require.Nil(t, JSONFieldFolds(nil))
	_ = foldFieldsTarget{unexported: ""}
}

// TestValidateUnambiguousJSONRootKeys rejects repeated typed members and
// non-canonical spellings of tagged members, and nothing else.
func TestValidateUnambiguousJSONRootKeys(t *testing.T) {
	t.Parallel()
	target := &foldParityTarget{}
	for _, body := range []string{
		`{"model":"a","Model":"b"}`,
		`{"model":"a","model":"b"}`,
		`{"model":"a","` + jsonEscapedForTest("m") + `odel":"b"}`,
		`{"extra_body":{},"EXTRA_BODY":{}}`,
		`{"max_tokens":1,"max_to` + jsonEscapedForTest(string(rune(0x212A))) + `ens":2}`,
	} {
		err := ValidateUnambiguousJSONRootKeys([]byte(body), target)
		require.ErrorIs(t, err, ErrAmbiguousJSONKey, body)
	}
	for _, body := range []string{
		`{"Model":"lone variant"}`,
		`{"MESSAGES":[]}`,
		`{"max_to` + jsonEscapedForTest(string(rune(0x212A))) + `ens":2}`,
	} {
		err := ValidateUnambiguousJSONRootKeys([]byte(body), target)
		require.ErrorIs(t, err, ErrAmbiguousJSONKey, body)
		require.Contains(t, err.Error(), "must be spelled", body)
	}
	for _, body := range []string{
		`{"model":"a","messages":[{"model":"b","Model":"c"}]}`,
		`{"model":"a","` + jsonEscapedForTest("model") + `x":1}`,
		`{"unknown":1,"unknown":2,"Unknown":3}`,
		`null`,
	} {
		require.NoError(t, ValidateUnambiguousJSONRootKeys([]byte(body), target), body)
	}
	// Untagged fields have no canonical wire spelling, so only duplicates count.
	untagged := &foldFieldsTarget{}
	require.NoError(t, ValidateUnambiguousJSONRootKeys([]byte(`{"untagged":"x","tagged":"y"}`), untagged))
	require.ErrorIs(t, ValidateUnambiguousJSONRootKeys([]byte(`{"untagged":"x","Untagged":"y"}`), untagged), ErrAmbiguousJSONKey)
	require.ErrorIs(t, ValidateUnambiguousJSONRootKeys([]byte(`{"Tagged":"y"}`), untagged), ErrAmbiguousJSONKey)
	var many strings.Builder
	many.WriteString(`{"model":"a"`)
	for range 1000 {
		many.WriteString(`,"MODEL":"b"`)
	}
	many.WriteString(`}`)
	err := ValidateUnambiguousJSONRootKeys([]byte(many.String()), target)
	require.ErrorIs(t, err, ErrAmbiguousJSONKey)
	require.Less(t, len(err.Error()), 600, "the diagnostic stays bounded")
	require.NoError(t, ValidateUnambiguousJSONRootKeys([]byte(`{"a":1,"a":2}`), &map[string]any{}))
	err = ValidateUnambiguousJSONRootKeys([]byte(`{"model":`), target)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrAmbiguousJSONKey)
}

// TestUnmarshalBodyReusableRejectsAmbiguousKeys checks the shared request
// decoder enforces the rule for JSON bodies and keeps form binding unchanged.
func TestUnmarshalBodyReusableRejectsAmbiguousKeys(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	newContext := func(contentType, body string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", contentType)
		return c
	}

	var ambiguous foldParityTarget
	err := UnmarshalBodyReusable(newContext("application/json", `{"model":"premium","Model":"cheap"}`), &ambiguous)
	require.True(t, errors.Is(err, ErrAmbiguousJSONKey), "got %v", err)

	var miscased foldParityTarget
	err = UnmarshalBodyReusable(newContext("application/json", `{"model":"cheap","Messages":[1]}`), &miscased)
	require.True(t, errors.Is(err, ErrAmbiguousJSONKey), "got %v", err)

	var plain foldParityTarget
	c := newContext("application/json; charset=utf-8", `{"model":"cheap","messages":[1],"vendor":{"Model":"x"}}`)
	require.NoError(t, UnmarshalBodyReusable(c, &plain))
	require.Equal(t, "cheap", plain.Model)
	require.Len(t, plain.Messages, 1)
	var again foldParityTarget
	require.NoError(t, UnmarshalBodyReusable(c, &again), "the cached body stays reusable")

	type formTarget struct {
		Model string `form:"model"`
	}
	var form formTarget
	require.NoError(t, UnmarshalBodyReusable(newContext("application/x-www-form-urlencoded", "model=a&model=b"), &form))
}

// referenceRootKeys lists root keys with encoding/json's tokenizer; it is the
// slow but obviously correct oracle for ScanJSONRootKeys.
func referenceRootKeys(body []byte) ([]string, bool) {
	if !json.Valid(body) {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return nil, false
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, false
	}
	var keys []string
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return nil, false
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, false
		}
	}
	return keys, true
}

// FuzzScanJSONRootKeys checks the byte walker against the tokenizer oracle on
// arbitrary input: identical keys for every valid object, an error otherwise.
func FuzzScanJSONRootKeys(f *testing.F) {
	for _, seed := range []string{
		`{}`, `{"a":1}`, `{"a":"\\","b":"\"}","c":["]",{"d":"\\\""}]}`,
		`{"k\"ey":1,"k\\":2,"` + jsonEscapedForTest("model") + `":3}`,
		` { "x" : { "y" : [ 1 , 2 ] } , "z" : null } `, `{"a":1}{"b":2}`,
		`{"\u00e9":1,"` + string([]byte{0xff}) + `":2}`, `[1]`, `"s"`, `{"a":tru}`,
		`{"n":-0.5e-3,"t":true,"f":false,"e":""}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		want, ok := referenceRootKeys(body)
		got, err := ScanJSONRootKeys(body)
		if !ok {
			require.Error(t, err)
			return
		}
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}
