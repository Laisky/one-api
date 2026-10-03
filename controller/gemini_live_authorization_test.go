package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
)

// TestGeminiLiveChannelUpdateRequiresOperator verifies the same AdminAuth gate
// used by /api/channel with signed sessions and the real update handler.
// Parameters: t is the test handle. Returns: none. Forged role/endpoint fields
// in a normal caller's request cannot turn channel administration into relay input.
func TestGeminiLiveChannelUpdateRequiresOperator(t *testing.T) {
	setupTokenAuthListModelsEnv(t)
	for _, tc := range []struct {
		name string
		role int
		want int
	}{
		{"anonymous", 0, http.StatusUnauthorized},
		{"ordinary_user", model.RoleCommonUser, http.StatusForbidden},
		{"operator_control", model.RoleAdminUser, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.role != 0 {
				// Persist distinct values for both unique user-token columns;
				// empty strings are real indexed values, not SQL NULL.
				require.NoError(t, model.DB.Create(&model.User{
					Id: 910 + tc.role, Username: "live-auth-" + tc.name,
					Password: "fixture", Role: tc.role, Status: model.UserStatusEnabled,
					AccessToken: fmt.Sprintf("%032d", 910+tc.role),
					AffCode:     fmt.Sprintf("live-auth-%d", tc.role),
				}).Error)
			}
			engine := gin.New()
			engine.Use(sessions.Sessions("live-test", cookie.NewStore([]byte("fixture-signing-key-not-a-real-secret"))))
			engine.Use(func(c *gin.Context) { gmw.SetLogger(c, logger.Logger); c.Next() })
			engine.GET("/__fixture/session", func(c *gin.Context) {
				session := sessions.Default(c)
				session.Set("username", "live-auth-"+tc.name)
				session.Set("role", tc.role)
				session.Set("id", 910+tc.role)
				session.Set("status", model.UserStatusEnabled)
				require.NoError(t, session.Save())
				c.Status(http.StatusNoContent)
			})
			reached := false
			engine.PUT("/api/channel", middleware.AdminAuth(), func(c *gin.Context) {
				reached = true
				UpdateChannel(c)
			})
			// Intentionally omit a channel reference: an authorized operator
			// reaches the real payload validator, but this test changes no config.
			request := httptest.NewRequest(http.MethodPut, "/api/channel?role=100", strings.NewReader(`{"role":100,"config":{"endpoint_urls":{"realtime":"wss://attacker.invalid/live"}}}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Role", "100")
			if tc.role != 0 {
				login := httptest.NewRecorder()
				engine.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/__fixture/session", nil))
				for _, c := range login.Result().Cookies() {
					request.AddCookie(c)
				}
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, request)
			t.Logf("role=%d status=%d update_handler_reached=%t", tc.role, w.Code, reached)
			if tc.want != 0 {
				require.Equal(t, tc.want, w.Code, fmt.Sprintf("response: %s", w.Body.String()))
			} else {
				require.NotEqual(t, http.StatusUnauthorized, w.Code)
				require.NotEqual(t, http.StatusForbidden, w.Code)
			}
			require.Equal(t, tc.role == model.RoleAdminUser, reached)
		})
	}
}
