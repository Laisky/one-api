package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestChannelPricingReadinessRouteRequiresAdministrator proves the channel
// editor's pricing readiness check is mounted on the shipped router under the
// channel admin group: anonymous callers and ordinary users are refused before
// the handler runs, while an administrator session reaches it.
func TestChannelPricingReadinessRouteRequiresAdministrator(t *testing.T) {
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
		role   int
		status int
	}{
		{name: "anonymous", status: http.StatusUnauthorized},
		{name: "ordinary-user", role: model.RoleCommonUser, status: http.StatusForbidden},
	} {
		t.Run(caller.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(sessions.Sessions("pricing-readiness-auth-test", cookie.NewStore([]byte("channel-readiness-test-cookie-secret"))))
			engine.Use(func(c *gin.Context) {
				gmw.SetLogger(c, logger.Logger)
				if caller.role != 0 {
					session := sessions.Default(c)
					session.Set("username", caller.name)
					session.Set("role", caller.role)
					session.Set("id", 987655)
					session.Set("status", model.UserStatusEnabled)
				}
			})
			require.NotPanics(t, func() { SetApiRouter(engine) })

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/channel/pricing-readiness", strings.NewReader(`{"type":`))
			request.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(recorder, request)
			require.Equal(t, caller.status, recorder.Code, recorder.Body.String())
			if caller.status == http.StatusForbidden {
				require.Contains(t, recorder.Body.String(), "insufficient permissions")
			}
		})
	}

	t.Run("administrator", func(t *testing.T) {
		_, engine, saved := csrfFixture(t)
		body := fmt.Sprintf(`{"type":%d,"models":"gpt-4o,gpt-6-cyber","model_mapping":null,"model_configs":null}`, channeltype.Azure)
		w := csrfRequest(engine, saved, http.MethodPost, "/api/channel/pricing-readiness", body, "https://gateway.test", "same-origin")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.JSONEq(t, `{"success":true,"message":"","data":{"gaps":[{"model":"gpt-6-cyber","upstream_model":"gpt-6-cyber","requirement":"token_tariff","message":"model \"gpt-6-cyber\" requires explicit positive input and output pricing in channel model_configs"}]}}`, w.Body.String())

		w = csrfRequest(engine, saved, http.MethodPost, "/api/channel/pricing-readiness", `{"type":`, "https://gateway.test", "same-origin")
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})
}
