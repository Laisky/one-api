package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
)

func TestMetricsAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		metricsToken   string
		authHeader     string
		expectedStatus int
		expectNext     bool
	}{
		{
			name:           "empty MetricsToken returns 403",
			metricsToken:   "",
			authHeader:     "",
			expectedStatus: http.StatusForbidden,
			expectNext:     false,
		},
		{
			name:           "no Authorization header returns 401",
			metricsToken:   "secret-token",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectNext:     false,
		},
		{
			name:           "wrong token returns 401",
			metricsToken:   "secret-token",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
			expectNext:     false,
		},
		{
			name:           "correct Bearer token returns 200",
			metricsToken:   "secret-token",
			authHeader:     "Bearer secret-token",
			expectedStatus: http.StatusOK,
			expectNext:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore MetricsToken
			originalToken := config.MetricsToken
			config.MetricsToken = tt.metricsToken
			defer func() { config.MetricsToken = originalToken }()

			nextCalled := false
			r := gin.New()
			r.GET("/metrics", MetricsAuth(), func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
			if nextCalled != tt.expectNext {
				t.Errorf("expected next handler called=%v, got %v", tt.expectNext, nextCalled)
			}
		})
	}
}
