package common

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// bodyFormatTarget binds the same field from JSON, form and query spellings.
type bodyFormatTarget struct {
	Model string `json:"model" form:"model"`
	N     int    `json:"n" form:"n"`
}

// newBodyFormatContext builds a request context whose query names "query-model"
// while the body carries its own values; an empty contentType omits the header.
func newBodyFormatContext(method, contentType, body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, "/v1/chat/completions?model=query-model&n=9", strings.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	return c
}

// TestUnmarshalBodyReusableNeverBindsQueryWhenBodyExists requires every request
// that carries a body to bind typed fields from that body, whatever its
// Content-Type says, because relay paths forward the body and not the query.
func TestUnmarshalBodyReusableNeverBindsQueryWhenBodyExists(t *testing.T) {
	const jsonBody = ` {"model":"body-model","n":2}`
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
		wantModel   string
		wantN       int
	}{
		{name: "json", contentType: "application/json", body: jsonBody, wantModel: "body-model", wantN: 2},
		{name: "json_mixed_case", contentType: "Application/JSON", body: jsonBody, wantModel: "body-model", wantN: 2},
		{name: "json_suffix", contentType: "application/vnd.api+json; charset=utf-8", body: jsonBody, wantModel: "body-model", wantN: 2},
		{name: "text_plain_json", contentType: "text/plain; charset=utf-8", body: jsonBody, wantModel: "body-model", wantN: 2},
		{name: "missing_json", contentType: "", body: jsonBody, wantModel: "body-model", wantN: 2},
		{name: "malformed_header_json", contentType: "text/plain; =broken", body: jsonBody, wantModel: "body-model", wantN: 2},
		{name: "urlencoded_form", contentType: "application/x-www-form-urlencoded", body: "model=form-model&n=3", wantModel: "form-model", wantN: 3},
		{name: "text_plain_non_json", contentType: "text/plain", body: "model=form-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newBodyFormatContext(http.MethodPost, tc.contentType, tc.body)
			var got bodyFormatTarget
			require.NoError(t, UnmarshalBodyReusable(c, &got))
			require.Equal(t, tc.wantModel, got.Model)
			require.Equal(t, tc.wantN, got.N)

			var again bodyFormatTarget
			require.NoError(t, UnmarshalBodyReusable(c, &again), "the cached body stays reusable")
			require.Equal(t, got, again)
		})
	}
}

// TestUnmarshalBodyReusableRejectsJSONLabeledAsForm requires a JSON object body
// labeled as a urlencoded form to be refused: the provider receives that label,
// and a body such as the polyglot below reads as different requests to a JSON
// parser and to a form parser.
func TestUnmarshalBodyReusableRejectsJSONLabeledAsForm(t *testing.T) {
	for _, body := range []string{
		`{"model":"body-model","n":2}`,
		"\r\n\t " + `{"model":"body-model"}`,
		`{"x":"&model=premium&n=12&","model":"cheap","n":1}`,
	} {
		for _, contentType := range []string{"application/x-www-form-urlencoded", "Application/X-WWW-Form-Urlencoded; charset=utf-8"} {
			c := newBodyFormatContext(http.MethodPost, contentType, body)
			var got bodyFormatTarget
			err := UnmarshalBodyReusable(c, &got)
			require.ErrorIs(t, err, ErrAmbiguousRequestBody, "content type %q body %q", contentType, body)
			require.Zero(t, got)
			require.False(t, IsJSONRequestBody(c, []byte(body)))
		}
	}
}

// TestUnmarshalBodyReusableMultipartReadsBodyOnly requires multipart payloads,
// including a differently cased media type, to bind fields from the body alone.
func TestUnmarshalBodyReusableMultipartReadsBodyOnly(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.WriteField("model", "multipart-model"))
	require.NoError(t, writer.Close())

	for _, contentType := range []string{writer.FormDataContentType(), strings.Replace(writer.FormDataContentType(), "multipart/form-data", "Multipart/Form-Data", 1)} {
		c := newBodyFormatContext(http.MethodPost, contentType, buf.String())
		var got bodyFormatTarget
		require.NoError(t, UnmarshalBodyReusable(c, &got))
		require.Equal(t, "multipart-model", got.Model)
		require.Zero(t, got.N, "the query must not fill fields the body omits")
	}
}

// TestUnmarshalBodyReusableKeepsQueryBindingWithoutBody requires GET and
// bodiless requests to keep gin's query binding, since no body is forwarded.
func TestUnmarshalBodyReusableKeepsQueryBindingWithoutBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		body   string
	}{
		{name: "get", method: http.MethodGet},
		{name: "get_with_body", method: http.MethodGet, body: `{"model":"body-model"}`},
		{name: "post_without_body", method: http.MethodPost},
		{name: "post_whitespace_body", method: http.MethodPost, body: " \n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newBodyFormatContext(tc.method, "", tc.body)
			var got bodyFormatTarget
			require.NoError(t, UnmarshalBodyReusable(c, &got))
			require.Equal(t, "query-model", got.Model)
			require.Equal(t, 9, got.N)
		})
	}
}

// TestIsJSONRequestBodyMatchesUnmarshalDecision requires the exported helper to
// agree with the reader UnmarshalBodyReusable picks for the same request.
func TestIsJSONRequestBodyMatchesUnmarshalDecision(t *testing.T) {
	require.True(t, IsJSONRequestBody(newBodyFormatContext(http.MethodPost, "text/plain", `{"model":"m"}`), []byte(`{"model":"m"}`)))
	require.True(t, IsJSONRequestBody(newBodyFormatContext(http.MethodPost, "application/json", ""), nil))
	require.False(t, IsJSONRequestBody(newBodyFormatContext(http.MethodPost, "application/x-www-form-urlencoded", "model=m"), []byte("model=m")))
	require.False(t, IsJSONRequestBody(newBodyFormatContext(http.MethodPost, "multipart/form-data; boundary=x", `{"model":"m"}`), []byte(`{"model":"m"}`)))
	require.False(t, IsJSONRequestBody(newBodyFormatContext(http.MethodGet, "", `{"model":"m"}`), []byte(`{"model":"m"}`)))
}
