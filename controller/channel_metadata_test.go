package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetChannelMetadata(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/api/channel/metadata", GetChannelMetadata)

	t.Run("returns default_base_url and base_url_editable for OpenAI", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest(http.MethodGet, "/api/channel/metadata?type=1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		require.True(t, response["success"].(bool))
		data := response["data"].(map[string]interface{})
		require.Equal(t, "https://api.openai.com", data["default_base_url"])
		require.True(t, data["base_url_editable"].(bool))
	})

	t.Run("returns non-editable for fixed base URL channels", func(t *testing.T) {
		t.Parallel()
		// Channel type 11 (PaLM) has a fixed base URL
		req, _ := http.NewRequest(http.MethodGet, "/api/channel/metadata?type=11", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		require.True(t, response["success"].(bool))
		data := response["data"].(map[string]interface{})
		require.Equal(t, "https://generativelanguage.googleapis.com", data["default_base_url"])
		require.False(t, data["base_url_editable"].(bool))
	})

	t.Run("returns error when type is missing", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest(http.MethodGet, "/api/channel/metadata", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		require.False(t, response["success"].(bool))
		require.Equal(t, "type is required", response["message"])
	})

	t.Run("returns error for invalid type", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest(http.MethodGet, "/api/channel/metadata?type=abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		require.False(t, response["success"].(bool))
		require.Equal(t, "invalid type", response["message"])
	})

	t.Run("returns empty values for out-of-range type", func(t *testing.T) {
		t.Parallel()
		req, _ := http.NewRequest(http.MethodGet, "/api/channel/metadata?type=9999", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		require.True(t, response["success"].(bool))
		data := response["data"].(map[string]interface{})
		require.Equal(t, "", data["default_base_url"])
		require.False(t, data["base_url_editable"].(bool))
	})
}
