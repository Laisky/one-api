package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestTurnstileCheckRedactsTokenFromRequestURL verifies downstream logs cannot read the raw Turnstile token from the live request URL.
func TestTurnstileCheckRedactsTokenFromRequestURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalTurnstile := config.TurnstileCheckEnabled
	config.TurnstileCheckEnabled = false
	t.Cleanup(func() {
		config.TurnstileCheckEnabled = originalTurnstile
	})

	router := gin.New()
	router.Use(TurnstileCheck())
	router.GET("/api/verification", func(c *gin.Context) {
		require.NotContains(t, c.Request.URL.String(), "secret-turnstile-token")
		require.Contains(t, c.Request.URL.String(), "turnstile=%5Bredacted%5D")
		require.Contains(t, c.Request.URL.String(), "email=user%40example.com")
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/verification?turnstile=secret-turnstile-token&email=user@example.com", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
}
