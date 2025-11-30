package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	metalib "github.com/songquanpeng/one-api/relay/meta"
)

func TestValidateImageRequest_DALLE3_RejectAutoQuality(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := []byte(`{"model":"dall-e-3","prompt":"p","size":"1024x1024","quality":"auto"}`)
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	ir, err := getImageRequest(c, 0)
	require.NoError(t, err, "getImageRequest error")

	// meta not used for validation currently
	got := validateImageRequest(ir, metalib.GetByContext(c), nil)
	require.NotNil(t, got, "expected validation error for quality=auto")
	require.Equal(t, http.StatusBadRequest, got.StatusCode)
}
