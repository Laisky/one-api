package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
)

// TestLogStrictOutResponses verifies log S2 response contracts across log list endpoints.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestLogStrictOutResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture, cleanup := setupUUIDContractTestEnvironment(t)
	t.Cleanup(cleanup)

	router := gin.New()
	router.GET("/api/log/", GetAllLogs)
	router.GET("/api/log/search", SearchAllLogs)
	router.GET("/api/log/self", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		GetUserLogs(c)
	})
	router.GET("/api/log/self/search", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		SearchUserLogs(c)
	})
	router.GET("/api/log/token", func(c *gin.Context) {
		c.Set(ctxkey.Id, fixture.user.Id)
		c.Set(ctxkey.TokenName, fixture.token.Name)
		GetTokenLogs(c)
	})

	tests := []struct {
		name string
		path string
	}{
		{name: "all", path: "/api/log/?p=0&size=10"},
		{name: "all_search", path: "/api/log/search?keyword=uuid%20contract&p=0&size=10"},
		{name: "self", path: "/api/log/self?p=0&size=10"},
		{name: "self_search", path: "/api/log/self/search?keyword=uuid%20contract&p=0&size=10"},
		{name: "token", path: "/api/log/token?p=0&size=10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
			require.Equal(t, true, payload["success"])
			rows, ok := payload["data"].([]any)
			require.True(t, ok, "log data must be an array: %s", recorder.Body.String())
			require.NotEmpty(t, rows)

			row, ok := rows[0].(map[string]any)
			require.True(t, ok, "log row must be an object")
			require.Equal(t, fixture.log.UUID, row["uuid"])
			require.Equal(t, fixture.user.UUID, row["user_uuid"])
			require.Equal(t, fixture.channel.UUID, row["channel_uuid"])
			require.Equal(t, fixture.channel.Name, row["channel_name"])
			require.Equal(t, fixture.token.UUID, row["token_uuid"])
			require.NotContains(t, row, "id")
			require.NotContains(t, row, "user_id")
			require.NotContains(t, row, "channel")
		})
	}
}
