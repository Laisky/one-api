package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// TestChannelResetRoutesRequireAdministrator uses the real shipped router with
// its real rate limiter and an isolated Redis fixture. Both anonymous callers
// and regular users must be denied before a batch handler touches the database.
func TestChannelResetRoutesRequireAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	originalRDB, originalEnabled := common.RDB, common.IsRedisEnabled()
	common.RDB = client
	common.SetRedisEnabled(true)
	t.Cleanup(func() {
		common.RDB = originalRDB
		common.SetRedisEnabled(originalEnabled)
		require.NoError(t, client.Close())
	})

	for _, caller := range []struct {
		name   string
		status int
	}{
		{name: "anonymous", status: http.StatusUnauthorized},
		{name: "ordinary-user", status: http.StatusForbidden},
	} {
		t.Run(caller.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(sessions.Sessions("reset-auth-test", cookie.NewStore([]byte("channel-reset-test-cookie-secret!"))))
			engine.Use(func(c *gin.Context) {
				gmw.SetLogger(c, logger.Logger)
				if caller.name == "ordinary-user" {
					session := sessions.Default(c)
					session.Set("username", "ordinary-user")
					session.Set("role", model.RoleCommonUser)
					session.Set("id", 987654)
					session.Set("status", model.UserStatusEnabled)
				}
			})
			require.NotPanics(t, func() { SetApiRouter(engine) })

			for _, path := range []string{
				"/api/channel/reset_models",
				"/api/channel/selection",
				"/api/channel/delete_selected_disabled",
				"/api/log/delete_selected",
				"/api/channel/018fcf6d-c484-7000-8000-000000000101/reset_models",
			} {
				t.Run(path, func(t *testing.T) {
					recorder := httptest.NewRecorder()
					engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
					require.Equal(t, caller.status, recorder.Code)
					if caller.status == http.StatusForbidden {
						require.Contains(t, recorder.Body.String(), "insufficient permissions")
					}
				})
			}
		})
	}
}
