package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// TestChannelResetRoutesRequireAdministrator uses the real shipped router and
// a regular user's session. Both mutations must be forbidden before touching the
// database; registering the full router also detects static/UUID route conflicts.
func TestChannelResetRoutesRequireAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("reset-auth-test", cookie.NewStore([]byte("channel-reset-test-cookie-secret!"))))
	engine.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		session := sessions.Default(c)
		session.Set("username", "ordinary-user")
		session.Set("role", model.RoleCommonUser)
		session.Set("id", 987654)
		session.Set("status", model.UserStatusEnabled)
	})
	require.NotPanics(t, func() { SetApiRouter(engine) })

	for _, path := range []string{
		"/api/channel/reset_models",
		"/api/channel/018fcf6d-c484-7000-8000-000000000101/reset_models",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
			require.Equal(t, http.StatusForbidden, recorder.Code)
			require.Contains(t, recorder.Body.String(), "insufficient permissions")
		})
	}
}
