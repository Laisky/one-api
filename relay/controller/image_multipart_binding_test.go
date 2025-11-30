package controller

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetImageRequest_MultipartBindsModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("prompt", "a cat"), "write prompt")
	require.NoError(t, writer.WriteField("model", "gpt-image-1"), "write model")
	require.NoError(t, writer.Close(), "close writer")

	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	imgReq, err := getImageRequest(c, 0)
	require.NoError(t, err, "getImageRequest error")
	require.NotNil(t, imgReq)
	require.Equal(t, "gpt-image-1", imgReq.Model)
}

func TestGetImageRequest_MultipartBindsModelMini(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("prompt", "a cat"), "write prompt")
	require.NoError(t, writer.WriteField("model", "gpt-image-1-mini"), "write model")
	require.NoError(t, writer.Close(), "close writer")

	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	imgReq, err := getImageRequest(c, 0)
	require.NoError(t, err, "getImageRequest error")
	require.NotNil(t, imgReq)
	require.Equal(t, "gpt-image-1-mini", imgReq.Model)
}
