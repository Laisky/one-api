package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// dashboard395FollowupRequest invokes the real dashboard handlers with an
// authenticated test identity. Parameters select the viewer, role and path;
// the return value is the decoded HTTP envelope, including rejected responses.
func dashboard395FollowupRequest(t *testing.T, userID, role int, path string) map[string]json.RawMessage {
	t.Helper()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		c.Set(ctxkey.Id, userID)
		c.Set(ctxkey.Role, role)
		c.Next()
	})
	router.GET("/api/user/dashboard", GetUserDashboard)
	router.GET("/api/user/dashboard/users", GetDashboardUsers)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

// TestDashboard395AdministratorReadAccess reproduces role=10 being offered a
// user selector but denied both its options and data. It verifies root parity
// for read-only reporting without changing ordinary-user ownership or caps.
func TestDashboard395AdministratorReadAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)
	requireDashboardCacheDisabled(t)
	withDashboardBudget(t, 0, time.Second)
	t.Cleanup(SetDashboardAggregateLifecycleContext(context.Background()))
	previousCap := config.DashboardMaxSitewideRangeDays
	config.DashboardMaxSitewideRangeDays = 30
	t.Cleanup(func() { config.DashboardMaxSitewideRangeDays = previousCap })

	other := &model.User{
		Username: "dashboard-395-other", Password: "hashed", AccessToken: "dashboard-395-other-token",
		AffCode: "dashboard-395-other-aff", Role: model.RoleCommonUser,
		Status: model.UserStatusEnabled, Group: "default",
	}
	require.NoError(t, model.DB.Create(other).Error)
	require.NoError(t, model.LOG_DB.Model(fixture.log).Updates(map[string]any{
		"created_at": int64(1767225600), "quota": 37, "model_name": "owner-model",
	}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: other.Id, UserUUID: &other.UUID, Username: other.Username,
		CreatedAt: 1767225600, Type: model.LogTypeConsume, ModelName: "other-model", Quota: 71,
	}).Error)

	for _, role := range []int{model.RoleAdminUser, model.RoleRootUser} {
		for _, scenario := range []struct {
			name, suffix string
			requests     int
			owner        string
		}{
			{"default site-wide", "", 2, ""},
			{"explicit site-wide", "&user_id=all", 2, ""},
			{"selected owner", "&user_id=" + fixture.user.UUID, 1, fixture.user.UUID},
			{"selected other owner", "&user_id=" + other.UUID, 1, other.UUID},
		} {
			t.Run(stringRole395(role)+"/"+scenario.name, func(t *testing.T) {
				response := dashboard395FollowupRequest(t, fixture.user.Id, role,
					"/api/user/dashboard?from_date=2026-01-01&to_date=2026-01-30"+scenario.suffix)
				require.JSONEq(t, "true", string(response["success"]), string(response["message"]))
				var data struct {
					Logs []struct { RequestCount int `json:"request_count"` } `json:"logs"`
					Users []struct { UUID string `json:"user_uuid"` } `json:"user_logs"`
				}
				require.NoError(t, json.Unmarshal(response["data"], &data))
				count := 0
				for _, row := range data.Logs { count += row.RequestCount }
				require.Equal(t, scenario.requests, count)
				if scenario.owner != "" {
					require.Len(t, data.Users, 1)
					require.Equal(t, scenario.owner, data.Users[0].UUID)
				}
			})
		}
		t.Run(stringRole395(role)+"/selector", func(t *testing.T) {
			response := dashboard395FollowupRequest(t, fixture.user.Id, role, "/api/user/dashboard/users")
			require.JSONEq(t, "true", string(response["success"]), string(response["message"]))
			var users []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(response["data"], &users))
			seen := map[string]bool{}
			for _, user := range users {
				var uuid string
				require.NoError(t, json.Unmarshal(user["uuid"], &uuid))
				seen[uuid] = true
				require.NotContains(t, user, "id")
				require.NotContains(t, user, "password")
				require.NotContains(t, user, "access_token")
			}
			require.True(t, seen["all"] && seen[fixture.user.UUID] && seen[other.UUID])
		})
		t.Run(stringRole395(role)+"/site-wide cap", func(t *testing.T) {
			response := dashboard395FollowupRequest(t, fixture.user.Id, role,
				"/api/user/dashboard?from_date=2026-01-01&to_date=2026-02-01&user_id=all")
			require.JSONEq(t, "false", string(response["success"]))
			require.Contains(t, string(response["message"]), "Site-wide dashboard range is limited")
		})
	}
	for _, path := range []string{
		"/api/user/dashboard/users",
		"/api/user/dashboard?from_date=2026-01-01&to_date=2026-01-07&user_id=all",
		"/api/user/dashboard?from_date=2026-01-01&to_date=2026-01-07&user_id="+other.UUID,
		"/api/user/dashboard?from_date=2026-01-01&to_date=2026-01-08",
	} {
		response := dashboard395FollowupRequest(t, fixture.user.Id, model.RoleCommonUser, path)
		require.JSONEq(t, "false", string(response["success"]), path)
		require.JSONEq(t, "null", string(response["data"]), path)
	}
	response := dashboard395FollowupRequest(t, fixture.user.Id, model.RoleCommonUser,
		"/api/user/dashboard?from_date=2026-01-01&to_date=2026-01-07")
	require.JSONEq(t, "true", string(response["success"]), string(response["message"]))
	var own struct { Users []struct { UUID string `json:"user_uuid"` } `json:"user_logs"` }
	require.NoError(t, json.Unmarshal(response["data"], &own))
	require.Len(t, own.Users, 1)
	require.Equal(t, fixture.user.UUID, own.Users[0].UUID)
}

// stringRole395 returns the stable test label for an administrator/root role.
func stringRole395(role int) string {
	if role == model.RoleAdminUser { return "admin" }
	return "root"
}
