package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestDashboard395ReadErrorsArePrivate injects database failures into each
// dashboard read path using t's isolated fixture. It returns nothing and checks
// the HTTP envelope, safe public text, one request-scoped diagnostic, and recovery.
func TestDashboard395ReadErrorsArePrivate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const dashboardPath = "/api/user/dashboard?from_date=2026-01-01&to_date=2026-01-07"
	for _, tc := range []struct {
		name, path, table, message string
		role                       int
	}{
		{"aggregates", dashboardPath, "logs", "Failed to get dashboard data", model.RoleCommonUser},
		{"site-wide quota", dashboardPath, "users", "Failed to get site-wide quota stats", model.RoleAdminUser},
		{"own quota", dashboardPath, "users", "Failed to get user data", model.RoleCommonUser},
		{"selector", "/api/user/dashboard/users", "users", "Failed to get user list", model.RoleAdminUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, restore := setupUUIDContractTestEnvironment(t)
			t.Cleanup(restore)
			db := model.DB
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			previousTTL := config.DashboardCacheTTLSec
			config.DashboardCacheTTLSec = 0
			t.Cleanup(func() { config.DashboardCacheTTLSec = previousTTL })
			withDashboardBudget(t, 0, time.Second)
			t.Cleanup(SetDashboardAggregateLifecycleContext(context.Background()))

			// A healthy request proves the fixture and selected handler work first.
			before := dashboard395FollowupRequest(t, fixture.user.Id, tc.role, tc.path)
			require.JSONEq(t, "true", string(before["success"]), string(before["message"]))
			failure := errors.New("fixture-only internal storage detail: " + tc.name)
			var hits atomic.Int32
			inject := func(tx *gorm.DB) {
				query := strings.NewReplacer("`", "", `"`, "").Replace(strings.ToLower(tx.Statement.SQL.String()))
				if tx.Statement.Table == tc.table || strings.Contains(query, "from "+tc.table) {
					hits.Add(1)
					tx.AddError(failure)
				}
			}

			// Capture the fixture registry, not whichever database a later test installs.
			queryCallbacks, rowCallbacks := db.Callback().Query(), db.Callback().Row()
			require.NoError(t, queryCallbacks.Before("gorm:query").Register("dashboard395:private_query", inject))
			t.Cleanup(func() { require.NoError(t, queryCallbacks.Remove("dashboard395:private_query")) })
			require.NoError(t, rowCallbacks.Before("gorm:row").Register("dashboard395:private_row", inject))
			t.Cleanup(func() { require.NoError(t, rowCallbacks.Remove("dashboard395:private_row")) })

			var diagnostics bytes.Buffer
			core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(&diagnostics), zapcore.DebugLevel)
			lg, err := glog.NewWithName("dashboard-review", glog.LevelDebug,
				zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }),
				zap.Fields(zap.String("fixture_request", tc.name)))
			require.NoError(t, err)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			c.Request = c.Request.WithContext(gmw.SetLogger(c, lg))
			c.Set(ctxkey.Id, fixture.user.Id)
			c.Set(ctxkey.Role, tc.role)
			if tc.name == "selector" {
				GetDashboardUsers(c)
			} else {
				GetUserDashboard(c)
			}
			require.Positive(t, hits.Load(), "the intended database failure must actually be injected")
			require.Equal(t, http.StatusOK, recorder.Code)
			var response map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, map[string]any{"success": false, "message": tc.message, "data": nil}, response)
			require.NotContains(t, recorder.Body.String(), failure.Error())

			// The internal cause remains diagnosable, once, on the request's logger.
			require.NotEmpty(t, diagnostics.String())
			lines := strings.Split(strings.TrimSpace(diagnostics.String()), "\n")
			require.Len(t, lines, 1, diagnostics.String())
			var entry map[string]any
			require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry))
			require.Equal(t, "error", entry["level"])
			require.Equal(t, strings.ToLower(tc.message), entry["msg"])
			require.Equal(t, tc.name, entry["fixture_request"])
			require.Contains(t, entry["error"], failure.Error())

			require.NoError(t, queryCallbacks.Remove("dashboard395:private_query"))
			require.NoError(t, rowCallbacks.Remove("dashboard395:private_row"))
			after := dashboard395FollowupRequest(t, fixture.user.Id, tc.role, tc.path)
			require.JSONEq(t, "true", string(after["success"]), string(after["message"]))
			require.JSONEq(t, string(before["data"]), string(after["data"]))
			t.Logf("DASHBOARD395_PRIVATE_ERROR path=%s injected=%d recovered=true", tc.name, hits.Load())
		})
	}
}

// TestTrace395CallbacksAreRemoved runs the actual trace regressions inside
// subtests and inspects their captured databases after cleanup has completed.
// Parameter t owns the fixtures; no value is returned. This checks registry
// lifecycle without falsely assuming separate gorm.Open calls share a registry.
func TestTrace395CallbacksAreRemoved(t *testing.T) {
	for _, tc := range []struct {
		name, query, row string
		run              func(*testing.T)
	}{
		{"read budget", "trace395:count_query", "trace395:count_row", TestTrace395ReadQueryBudget},
		{"log dependency", "issue395:query_budget", "issue395:raw_budget", TestTrace395AdminCorrelationReadAvoidsLogDependency},
		{"cancellation", "issue395:query_context", "issue395:raw_context", TestTrace395LogReadCancellation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var db *gorm.DB
			require.True(t, t.Run("exercise", func(t *testing.T) {
				tc.run(t)
				db = model.DB // capture before the existing fixture restores the globals
			}))
			require.NotNil(t, db)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.Nil(t, db.Callback().Query().Get(tc.query), "query callback survived its test")
			require.Nil(t, db.Callback().Row().Get(tc.row), "row callback survived its test")
			var log model.Log
			require.NoError(t, db.First(&log).Error)
			var id int
			require.NoError(t, db.Raw("SELECT id FROM logs LIMIT 1").Scan(&id).Error)
			require.Equal(t, log.Id, id)
			t.Logf("TRACE395_CALLBACK_CLEANUP query=%s row=%s subsequent_reads=ok", tc.query, tc.row)
		})
	}
}
