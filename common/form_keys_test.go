package common

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateFormKeysRejectsRepeatedScalars pins the HTTP parameter pollution
// rule: a scalar field may occur once in one spelling, while the documented
// array fields may repeat. Parameters: t is a test. Returns: none.
func TestValidateFormKeysRejectsRepeatedScalars(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		counts map[string]int
		reject bool
	}{
		{name: "single_values", counts: map[string]int{"model": 1, "prompt": 1, "n": 1}},
		{name: "repeated_scalar", counts: map[string]int{"model": 2}, reject: true},
		{name: "case_variant", counts: map[string]int{"model": 1, "Model": 1}, reject: true},
		{name: "bracket_variant", counts: map[string]int{"model": 1, "model[]": 1}, reject: true},
		{name: "repeated_bracket_scalar", counts: map[string]int{"seconds[]": 2}, reject: true},
		{name: "long_s_fold", counts: map[string]int{"size": 1, "\u017fize": 1}, reject: true},
		{name: "kelvin_fold", counts: map[string]int{"kind": 1, "\u212aind": 1}, reject: true},
		{name: "bracket_alias_of_scalar", counts: map[string]int{"seconds[]": 1}, reject: true},
		{name: "control_character", counts: map[string]int{"model\x00": 1}, reject: true},
		{name: "c1_control_character", counts: map[string]int{"model\u0085": 1}, reject: true},
		{name: "single_array_bracket", counts: map[string]int{"include[]": 1}},
		{name: "image_array", counts: map[string]int{"image[]": 3, "prompt": 1}},
		{name: "image_repeated_plain", counts: map[string]int{"image": 2}},
		{name: "transcription_arrays", counts: map[string]int{"include[]": 10, "timestamp_granularities[]": 2, "known_speaker_names[]": 4, "known_speaker_references[]": 4, "languages[]": 2, "keywords[]": 2}},
		{name: "distinct_object_members", counts: map[string]int{"chunking_strategy[type]": 1, "chunking_strategy[threshold]": 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFormKeys(tc.counts)
			if tc.reject {
				require.ErrorIs(t, err, ErrAmbiguousFormKey)
				require.True(t, IsAmbiguousRequestError(err))
				return
			}
			require.NoError(t, err)
		})
	}

	counts := map[string]int{}
	for i := range 20 {
		counts[strings.Repeat(string(rune('a'+i)), 100)] = 2
	}
	err := validateFormKeys(counts)
	require.ErrorIs(t, err, ErrAmbiguousFormKey)
	require.Less(t, len(err.Error()), 1024, "client-chosen names in the message stay bounded")
}

// TestUnmarshalBodyReusableRejectsRepeatedFormFields drives the shared decoder
// with urlencoded and multipart bodies, including a repeated file part.
// Parameters: t is a test. Returns: none.
func TestUnmarshalBodyReusableRejectsRepeatedFormFields(t *testing.T) {
	type target struct {
		Model string `form:"model"`
	}
	for _, body := range []string{"model=cheap&model=premium", "model=cheap&Model=premium", "model=cheap&model%5B%5D=premium"} {
		var got target
		err := UnmarshalBodyReusable(newBodyFormatContext(http.MethodPost, "application/x-www-form-urlencoded", body), &got)
		require.ErrorIs(t, err, ErrAmbiguousFormKey, body)
	}

	build := func(parts func(w *multipart.Writer)) (string, string) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		parts(w)
		require.NoError(t, w.Close())
		return w.FormDataContentType(), buf.String()
	}
	contentType, body := build(func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("model", "cheap"))
		require.NoError(t, w.WriteField("model", "premium"))
	})
	var got target
	require.ErrorIs(t, UnmarshalBodyReusable(newBodyFormatContext(http.MethodPost, contentType, body), &got), ErrAmbiguousFormKey)

	contentType, body = build(func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("model", "cheap"))
		for range 2 {
			file, err := w.CreateFormFile("file", "a.wav")
			require.NoError(t, err)
			_, err = file.Write([]byte("RIFF"))
			require.NoError(t, err)
		}
	})
	require.ErrorIs(t, UnmarshalBodyReusable(newBodyFormatContext(http.MethodPost, contentType, body), &got), ErrAmbiguousFormKey, "a repeated file part is a repeated scalar")

	contentType, body = build(func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("model", "gpt-image-1"))
		for range 2 {
			file, err := w.CreateFormFile("image[]", "a.png")
			require.NoError(t, err)
			_, err = file.Write([]byte("PNG"))
			require.NoError(t, err)
		}
		require.NoError(t, w.WriteField("include[]", "logprobs"))
		require.NoError(t, w.WriteField("include[]", "segments"))
	})
	got = target{}
	require.NoError(t, UnmarshalBodyReusable(newBodyFormatContext(http.MethodPost, contentType, body), &got), "documented array fields may repeat")
	require.Equal(t, "gpt-image-1", got.Model)
}

// TestUnmarshalBodyReusableRejectsMultipartPartsOtherParsersRead verifies that
// a multipart part Go's form parser would drop or decode differently, but
// that other parsers still read as a field, cannot smuggle a second value.
// Parameters: t is a test. Returns: none.
func TestUnmarshalBodyReusableRejectsMultipartPartsOtherParsersRead(t *testing.T) {
	type target struct {
		Model string `form:"model"`
	}
	for _, tc := range []struct {
		name   string
		header textproto.MIMEHeader
	}{
		{name: "duplicate_name_parameter", header: textproto.MIMEHeader{"Content-Disposition": {`form-data; name="x"; name="model"`}}},
		{name: "attachment_disposition", header: textproto.MIMEHeader{"Content-Disposition": {`attachment; name="model"`}}},
		{name: "extended_name_parameter", header: textproto.MIMEHeader{"Content-Disposition": {`form-data; name="x"; name*=iso-8859-1''model`}}},
		{name: "continued_name_parameter", header: textproto.MIMEHeader{"Content-Disposition": {`form-data; name*0="mo"; name*1="del"`}}},
		{name: "two_dispositions", header: textproto.MIMEHeader{"Content-Disposition": {`form-data; name="x"`, `form-data; name="model"`}}},
		{name: "missing_disposition", header: textproto.MIMEHeader{"Content-Type": {"text/plain"}}},
		{name: "quoted_printable", header: textproto.MIMEHeader{"Content-Disposition": {`form-data; name="note"`}, "Content-Transfer-Encoding": {"quoted-printable"}}},
		{name: "backslash_escaped_name", header: textproto.MIMEHeader{"Content-Disposition": {`form-data; name="\n"`}}},
		{name: "folded_disposition", header: textproto.MIMEHeader{"Content-Disposition": {"form-data;\r\n name=\"note\""}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := multipart.NewWriter(&buf)
			require.NoError(t, w.WriteField("model", "cheap"))
			part, err := w.CreatePart(tc.header)
			require.NoError(t, err)
			_, err = part.Write([]byte("premium"))
			require.NoError(t, err)
			require.NoError(t, w.Close())

			var got target
			err = UnmarshalBodyReusable(newBodyFormatContext(http.MethodPost, w.FormDataContentType(), buf.String()), &got)
			require.ErrorIs(t, err, ErrAmbiguousFormKey)
		})
	}
}

// TestUnmarshalBodyReusableMultipartBoundaryAndFilenameEncodings verifies an
// escaped boundary is refused while an extended filename, which names no
// field, stays accepted. Parameters: t is a test. Returns: none.
func TestUnmarshalBodyReusableMultipartBoundaryAndFilenameEncodings(t *testing.T) {
	type target struct {
		Model string `form:"model"`
	}
	escaped := "--a\\b\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\ncheap\r\n--a\\b--\r\n"
	var got target
	err := UnmarshalBodyReusable(newBodyFormatContext(http.MethodPost, `multipart/form-data; boundary="a\\b"`, escaped), &got)
	require.ErrorIs(t, err, ErrAmbiguousFormKey)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	require.NoError(t, w.WriteField("model", "whisper-1"))
	part, err := w.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename*=utf-8''a.wav`}})
	require.NoError(t, err)
	_, err = part.Write([]byte("RIFF"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	got = target{}
	require.NoError(t, UnmarshalBodyReusable(newBodyFormatContext(http.MethodPost, w.FormDataContentType(), buf.String()), &got))
	require.Equal(t, "whisper-1", got.Model)
}
