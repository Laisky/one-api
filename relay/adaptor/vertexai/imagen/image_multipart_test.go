package imagen

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestConvertMultipartImageEditRequest ensures we can parse required fields from multipart form
// and construct an Imagen create request without panicking.
func TestConvertMultipartImageEditRequest(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()

	// Build a multipart body with image, mask, prompt, model and response_format=b64_json
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// required file parts
	imgPart, err := writer.CreateFormFile("image", "img.png")
	require.NoError(t, err, "create image part")
	_, _ = imgPart.Write([]byte("PNG"))

	maskPart, err := writer.CreateFormFile("mask", "mask.png")
	require.NoError(t, err, "create mask part")
	_, _ = maskPart.Write([]byte("PNG"))

	// required fields
	require.NoError(t, writer.WriteField("prompt", "Edit this image"), "write prompt")
	require.NoError(t, writer.WriteField("model", "imagen-3.0"), "write model")
	require.NoError(t, writer.WriteField("response_format", "b64_json"), "write response_format")

	require.NoError(t, writer.Close(), "close writer")

	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/images/edits", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	converted, err := ConvertMultipartImageEditRequest(c)
	require.NoError(t, err, "ConvertMultipartImageEditRequest error")
	require.NotNil(t, converted, "expected non-nil converted request")
	require.NotEmpty(t, converted.Instances, "expected converted request with instances")
}
