package middleware

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestSecurityDashboardBearerControl preserves current-database bearer access
// without creating a new cookie or applying browser-only role restrictions.
func TestSecurityDashboardBearerControl(t *testing.T) {
	db := sessionSafetyDatabase(t)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 971).Update("access_token", "synthetic-dashboard-bearer").Error)
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	for _, valid := range []bool{false, true} {
		request := httptest.NewRequest(http.MethodGet, "/root", nil)
		token := "invalid-fixture"
		want := http.StatusUnauthorized
		if valid {
			token = "synthetic-dashboard-bearer"
			want = http.StatusNoContent
		}
		request.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		sessionSafetyRouter(key, 971).ServeHTTP(w, request)
		require.Equal(t, want, w.Code)
		require.Empty(t, w.Header().Values("Set-Cookie"))
	}
}

// TestSecurityDashboardSessionRoleCeiling requires a new login for promotions
// and supplies the same effective role through both public context views.
func TestSecurityDashboardSessionRoleCeiling(t *testing.T) {
	sessionSafetyDatabase(t)
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(sessions.Sessions("session-safety", cookie.NewStore(key)))
	engine.POST("/issue-fixture", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("id", 971)
		s.Set("username", "session-fixture")
		s.Set("role", model.RoleCommonUser)
		s.Set("status", model.UserStatusEnabled)
		require.NoError(t, s.Save())
		c.Status(http.StatusNoContent)
	})
	engine.GET("/root", RootAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	engine.GET("/optional", OptionalUserAuth(), func(c *gin.Context) {
		require.Equal(t, model.RoleCommonUser, c.GetInt(ctxkey.Role))
		value, exists := c.Get(ctxkey.UserObj)
		require.True(t, exists)
		require.Equal(t, model.RoleCommonUser, value.(*model.User).Role)
		c.Status(http.StatusNoContent)
	})
	saved := sessionSafetyCookie(t, engine)
	for _, path := range []string{"/root", "/optional"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(saved)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, request)
		want := http.StatusNoContent
		if path == "/root" {
			want = http.StatusForbidden
		}
		require.Equal(t, want, w.Code)
	}
}
