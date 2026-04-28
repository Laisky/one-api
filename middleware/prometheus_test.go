package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestMetricsAuth verifies that the MetricsAuth middleware correctly enforces
// Bearer token authentication on the /metrics endpoint across all branches.
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

			require.Equal(t, tt.expectedStatus, w.Code, "unexpected status code")
			require.Equal(t, tt.expectNext, nextCalled, "unexpected next-handler invocation")
		})
	}
}
