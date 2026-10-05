package middleware

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// sessionSafetyDatabase creates an isolated actual user table, not an identity stub.
func sessionSafetyDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "accounts.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldRedis, oldSQLite, oldMode := model.DB, common.IsRedisEnabled(), common.UsingSQLite.Load(), gin.Mode()
	model.DB = db
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(true)
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		model.DB = oldDB
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		gin.SetMode(oldMode)
		_ = sqlDB.Close()
	})
	require.NoError(t, db.Create(&model.User{Id: 971, UUID: "018f0000-0000-7000-8000-000000000971", Username: "session-fixture", Password: "unused-test-hash", Role: model.RoleRootUser, Status: model.UserStatusEnabled, Group: "default"}).Error)
	return db
}

// sessionSafetyRouter uses real cookie signing/verification and production middleware.
func sessionSafetyRouter(key []byte, id any) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery(), sessions.Sessions("session-safety", cookie.NewStore(key)))
	engine.POST("/issue-fixture", func(c *gin.Context) {
		s := sessions.Default(c)
		s.Set("id", id)
		s.Set("username", "session-fixture")
		s.Set("role", model.RoleRootUser)
		s.Set("status", model.UserStatusEnabled)
		if err := s.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	engine.GET("/root", RootAuth(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	engine.GET("/optional", OptionalUserAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.GetInt(ctxkey.Id), "role": c.GetInt(ctxkey.Role)})
	})
	return engine
}

// sessionSafetyCookie issues a cookie before later privilege/account changes.
func sessionSafetyCookie(t *testing.T, engine *gin.Engine) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/issue-fixture", nil))
	require.Equal(t, http.StatusNoContent, w.Code)
	cookies := w.Result().Cookies()
	require.NotEmpty(t, cookies)
	return cookies[0]
}

// TestSecurityDashboardSessionCurrentAccount verifies old role/status claims can
// neither outlive revocation nor supply authority after account lookup failures.
func TestSecurityDashboardSessionCurrentAccount(t *testing.T) {
	for _, scenario := range []string{"enabled_control", "demoted", "disabled", "deleted", "database_failure", "uninitialized_database", "malformed_id"} {
		t.Run(scenario, func(t *testing.T) {
			db := sessionSafetyDatabase(t)
			key := make([]byte, 32)
			_, err := rand.Read(key)
			require.NoError(t, err)
			var id any = 971
			if scenario == "malformed_id" {
				id = "971"
			}
			engine := sessionSafetyRouter(key, id)
			saved := sessionSafetyCookie(t, engine)
			wantRoot, wantID, wantRole := http.StatusNoContent, 971, model.RoleRootUser
			switch scenario {
			case "demoted":
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", 971).Update("role", model.RoleCommonUser).Error)
				wantRoot = http.StatusForbidden
				wantRole = model.RoleCommonUser
			case "disabled":
				require.NoError(t, db.Model(&model.User{}).Where("id = ?", 971).Update("status", model.UserStatusDisabled).Error)
				wantRoot = http.StatusForbidden
				wantID = 0
				wantRole = 0
			case "deleted":
				require.NoError(t, db.Delete(&model.User{}, 971).Error)
				wantRoot = http.StatusUnauthorized
				wantID = 0
				wantRole = 0
			case "database_failure":
				require.NoError(t, db.Migrator().DropTable(&model.User{}))
				wantRoot = http.StatusUnauthorized
				wantID = 0
				wantRole = 0
			case "uninitialized_database":
				model.DB = nil
				wantRoot = http.StatusUnauthorized
				wantID = 0
				wantRole = 0
			case "malformed_id":
				wantRoot = http.StatusUnauthorized
				wantID = 0
				wantRole = 0
			}
			request := httptest.NewRequest(http.MethodGet, "/root", nil)
			request.AddCookie(saved)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, request)
			t.Logf("CURRENT_SESSION scenario=%s root_status=%d want=%d", scenario, w.Code, wantRoot)
			require.Equal(t, wantRoot, w.Code)
			request = httptest.NewRequest(http.MethodGet, "/optional", nil)
			request.AddCookie(saved)
			w = httptest.NewRecorder()
			engine.ServeHTTP(w, request)
			require.Equal(t, http.StatusOK, w.Code)
			var result struct {
				ID   int `json:"id"`
				Role int `json:"role"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
			require.Equal(t, wantID, result.ID)
			require.Equal(t, wantRole, result.Role)
		})
	}
}

// TestSecurityDashboardSessionSecretRotation retains same-key replica access
// while a new independently generated key invalidates the old cookie.
func TestSecurityDashboardSessionSecretRotation(t *testing.T) {
	sessionSafetyDatabase(t)
	first, second := make([]byte, 32), make([]byte, 32)
	_, err := rand.Read(first)
	require.NoError(t, err)
	_, err = rand.Read(second)
	require.NoError(t, err)
	saved := sessionSafetyCookie(t, sessionSafetyRouter(first, 971))
	for _, tc := range []struct {
		name string
		key  []byte
		want int
	}{{"same_key_replica", first, http.StatusNoContent}, {"rotated_key", second, http.StatusUnauthorized}} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/root", nil)
			request.AddCookie(saved)
			w := httptest.NewRecorder()
			sessionSafetyRouter(tc.key, 971).ServeHTTP(w, request)
			require.Equal(t, tc.want, w.Code)
		})
	}
}
