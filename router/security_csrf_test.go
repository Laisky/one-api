package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// csrfFixture installs a real account database and production API routes, returning
// the database, router and a signed cookie for an enabled administrator.
func csrfFixture(t *testing.T) (*gorm.DB, *gin.Engine, *http.Cookie) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "csrf.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldRedis, oldSQLite := model.DB, common.IsRedisEnabled(), common.UsingSQLite.Load()
	oldRate, oldServer, oldFrontend := config.RateLimitDisabled, config.ServerAddress, config.FrontendBaseURL
	oldSecure, oldClient, oldMode := config.EnableCookieSecure, client.HTTPClient, gin.Mode()
	model.DB = db
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(true)
	config.RateLimitDisabled = true
	config.ServerAddress = "https://gateway.test"
	config.FrontendBaseURL = ""
	config.EnableCookieSecure = true
	client.HTTPClient = &http.Client{}
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		model.DB = oldDB
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.RateLimitDisabled, config.ServerAddress, config.FrontendBaseURL = oldRate, oldServer, oldFrontend
		config.EnableCookieSecure, client.HTTPClient = oldSecure, oldClient
		gin.SetMode(oldMode)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Create(&model.User{Id: 971, UUID: "018f0000-0000-7000-8000-000000000971", Username: "csrf-fixture", Password: "unused-test-password", DisplayName: "original", AccessToken: "fixture-access-token", Role: model.RoleRootUser, Status: model.UserStatusEnabled, Group: "default"}).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { gmw.SetLogger(c, logger.Logger) })
	store := cookie.NewStore([]byte("csrf-fixture-cookie-signing-secret"))
	store.Options(sessions.Options{Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	engine.Use(sessions.Sessions("csrf-fixture", store))
	engine.POST("/issue-fixture", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 971)
		session.Set("username", "csrf-fixture")
		session.Set("role", model.RoleRootUser)
		session.Set("status", model.UserStatusEnabled)
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	SetApiRouter(engine)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "https://gateway.test/issue-fixture", nil))
	require.Equal(t, http.StatusNoContent, w.Code)
	saved := w.Result().Cookies()
	require.Len(t, saved, 1)
	return db, engine, saved[0]
}

// csrfRequest dispatches an API request with optional signed-cookie credentials
// and returns the recorder so callers can inspect status and side effects.
func csrfRequest(engine *gin.Engine, saved *http.Cookie, method, path, body, origin, site string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://gateway.test"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	request.Header.Set("Sec-Fetch-Site", site)
	if saved != nil {
		request.AddCookie(saved)
	} else {
		request.Header.Set("Authorization", "Bearer fixture-access-token")
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, request)
	return w
}

// TestSecurityCSRFProfileMutation exercises actual persisted account updates,
// including valid same-origin and bearer controls that rule out broken fixtures.
func TestSecurityCSRFProfileMutation(t *testing.T) {
	for _, tc := range []struct {
		name, origin, site string
		bearer             bool
		want               int
	}{
		{"same_origin", "https://gateway.test", "same-origin", false, 200},
		{"bearer_only", "https://evil.test", "cross-site", true, 200},
		{"cross_site", "https://evil.test", "cross-site", false, 403},
		{"sibling_site", "https://sibling.gateway.test", "same-site", false, 403},
		{"missing_provenance", "", "", false, 403},
		{"null_origin", "null", "same-origin", false, 403},
		{"wrong_scheme", "http://gateway.test", "same-site", false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, engine, saved := csrfFixture(t)
			if tc.bearer {
				saved = nil
			}
			w := csrfRequest(engine, saved, http.MethodPut, "/api/user/self", `{"display_name":"changed"}`, tc.origin, tc.site)
			var user model.User
			require.NoError(t, db.First(&user, 971).Error)
			if tc.want == 200 {
				require.Equal(t, "changed", user.DisplayName)
			} else {
				require.Equal(t, "original", user.DisplayName, "hostile request changed persisted account state")
			}
			require.Equal(t, tc.want, w.Code)
		})
	}
}

// TestSecurityCSRFTokenNavigation verifies a Lax-cookie navigation cannot rotate
// credentials through the production GET route, while account reads still work.
func TestSecurityCSRFTokenNavigation(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	control := csrfRequest(engine, saved, http.MethodGet, "/api/user/self", "", "", "same-origin")
	require.Equal(t, http.StatusOK, control.Code)
	require.Contains(t, control.Body.String(), "csrf-fixture")
	csrfRequest(engine, saved, http.MethodGet, "/api/user/token", "", "", "cross-site")
	var user model.User
	require.NoError(t, db.First(&user, 971).Error)
	require.Equal(t, "fixture-access-token", user.AccessToken)
}

// TestSecurityCSRFBalanceNavigation proves hostile navigation cannot dispatch to
// a local upstream or persist its balance, avoiding paid or external requests.
func TestSecurityCSRFBalanceNavigation(t *testing.T) {
	db, engine, saved := csrfFixture(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"total_available":42}`))
		require.NoError(t, err)
	}))
	t.Cleanup(upstream.Close)
	base := upstream.URL
	require.NoError(t, db.Create(&model.Channel{Id: 972, UUID: "018f0000-0000-7000-8000-000000000972", Name: "local-balance", Type: channeltype.CloseAI, Key: "fixture-only-key", BaseURL: &base, Status: model.ChannelStatusEnabled, Balance: 1}).Error)
	control := csrfRequest(engine, saved, http.MethodGet, "/api/channel/update_balance/018f0000-0000-7000-8000-000000000972", "", "https://gateway.test", "same-origin")
	require.Equal(t, http.StatusOK, control.Code)
	require.Contains(t, control.Body.String(), `"success":true`)
	require.Equal(t, int32(1), calls.Load())
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 972).Update("balance", 1).Error)
	calls.Store(0)
	csrfRequest(engine, saved, http.MethodGet, "/api/channel/update_balance/018f0000-0000-7000-8000-000000000972", "", "", "cross-site")
	var channel model.Channel
	require.NoError(t, db.First(&channel, 972).Error)
	require.Equal(t, int32(0), calls.Load(), "hostile GET dispatched to the upstream")
	require.Equal(t, float64(1), channel.Balance)
}
