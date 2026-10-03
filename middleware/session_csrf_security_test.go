package middleware

import (
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

// TestSecuritySessionMutationOrigin verifies cross-origin cookie-authenticated mutations stop before handlers.
func TestSecuritySessionMutationOrigin(t *testing.T) {
	db := setupTokenAuthChannelSuffixTestDB(t)
	old := model.DB
	model.DB = db
	redis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { model.DB = old; common.SetRedisEnabled(redis) })
	r := gin.New()
	r.Use(sessions.Sessions("fixture", cookie.NewStore([]byte("fixture-session-signing-secret"))))
	r.GET("/login", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("username", "admin")
		s.Set("id", 1)
		s.Set("role", model.RoleAdminUser)
		s.Set("status", model.UserStatusEnabled)
		require.NoError(t, s.Save())
		c.Status(204)
	})
	r.POST("/mutate", AdminAuth(), func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "https://gateway.test/login", nil))
	cookies := w.Result().Cookies()
	require.NotEmpty(t, cookies)
	for _, tc := range []struct {
		origin, site string
		want         int
	}{{"https://other.test", "cross-site", 403}, {"https://sibling.gateway.test", "same-site", 403}, {"https://gateway.test", "same-origin", 204}, {"", "", 204}} {
		req := httptest.NewRequest("POST", "https://gateway.test/mutate", nil)
		req.AddCookie(cookies[0])
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, tc.want, w.Code, tc.origin)
	}
}
