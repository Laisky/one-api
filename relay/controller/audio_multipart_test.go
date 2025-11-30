package controller

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestExtractAudioModelFromMultipart verifies that we correctly parse the `model` field
// from multipart/form-data for audio transcription/translation requests.
func TestExtractAudioModelFromMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()

	// Build a multipart body with a dummy file field and model
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	// file part (content is irrelevant for this test)
	fw, err := writer.CreateFormFile("file", "a.mp3")
	require.NoError(t, err, "create form file")
	_, _ = fw.Write([]byte("dummy"))

	// model field
	require.NoError(t, writer.WriteField("model", "gpt-4o-mini-transcribe"), "write field")
	require.NoError(t, writer.Close(), "close writer")

	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/audio/transcriptions", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	got := extractAudioModelFromMultipart(c)
	require.Equal(t, "gpt-4o-mini-transcribe", got)
}
