package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// imageEditBodyContext returns a gin context whose request carries body under
// contentType. Parameters: t owns the fixture, contentType and body are the
// client request. Returns: the context.
func imageEditBodyContext(t *testing.T, contentType string, body []byte) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	return c
}

// TestRewriteImageEditBody covers the edit body rewrite's edge cases: every
// spelling of a rewritten or omitted JSON key is replaced, a multipart body
// without a model part gains one, a truncated multipart body is refused, and a
// body that is neither JSON nor multipart is left to the caller. Parameters: t
// runs the cases. Returns: none.
func TestRewriteImageEditBody(t *testing.T) {
	t.Run("json_replaces_every_spelling", func(t *testing.T) {
		raw := []byte(`{"Model":"alias","prompt":"p","SAFETY_IDENTIFIER":"u","images":[{"file_id":"f"}]}`)
		c := imageEditBodyContext(t, "application/json", raw)
		out, ok, err := rewriteImageEditBody(c, raw, "gpt-image-2.5-sunburst", azureImageEditOmittedFields)
		require.NoError(t, err)
		require.True(t, ok)
		var sent map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out, &sent))
		require.Equal(t, map[string]json.RawMessage{
			"model":  json.RawMessage(`"gpt-image-2.5-sunburst"`),
			"prompt": json.RawMessage(`"p"`),
			"images": json.RawMessage(`[{"file_id":"f"}]`),
		}, sent)
	})

	t.Run("multipart_without_model_gains_one", func(t *testing.T) {
		raw, boundary := buildTestMultipart(t,
			testMultipartPart{name: "prompt", content: []byte("p")},
			testMultipartPart{name: "image", filename: "a.png", content: []byte("png-bytes")},
		)
		contentType := "multipart/form-data; boundary=" + boundary
		c := imageEditBodyContext(t, contentType, raw)
		out, ok, err := rewriteImageEditBody(c, raw, "gpt-image-2.5-flare", nil)
		require.NoError(t, err)
		require.True(t, ok)
		parts := readImageEditWireParts(t, contentType, out)
		require.Equal(t, []imageEditWirePart{
			{name: "model", content: []byte("gpt-image-2.5-flare")},
			{name: "prompt", content: []byte("p")},
			{name: "image", filename: "a.png", content: []byte("png-bytes")},
		}, parts)
	})

	t.Run("truncated_multipart_is_refused", func(t *testing.T) {
		raw, boundary := buildTestMultipart(t, testMultipartPart{name: "prompt", content: []byte("p")})
		truncated := raw[:len(raw)-len("--\r\n")-len(boundary)-4]
		c := imageEditBodyContext(t, "multipart/form-data; boundary="+boundary, truncated)
		_, ok, err := rewriteImageEditBody(c, truncated, "gpt-image-2.5-flare", nil)
		require.Error(t, err)
		require.True(t, ok)
	})

	t.Run("urlencoded_is_left_to_the_caller", func(t *testing.T) {
		raw := []byte("model=alias&prompt=p")
		c := imageEditBodyContext(t, "application/x-www-form-urlencoded", raw)
		out, ok, err := rewriteImageEditBody(c, raw, "gpt-image-2.5-flare", nil)
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, out)
	})
}
