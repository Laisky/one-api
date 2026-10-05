package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestSecurityAdminTokenInventory verifies list, search, detail and pagination
// expose root-owned token metadata but never return usable owner credentials.
func TestSecurityAdminTokenInventory(t *testing.T) {
	cleanup, user, token := setupConsumeTokenTest(t)
	defer cleanup()
	require.NoError(t, model.DB.Model(user).Update("role", model.RoleRootUser).Error)
	r := gin.New()
	r.GET("/list", AdminGetAllTokens)
	r.GET("/search", AdminSearchTokens)
	r.GET("/detail/:id", AdminGetToken)
	r.GET("/owner/:id", func(c *gin.Context) { c.Set("id", token.UserId); GetToken(c) })
	for _, path := range []string{"/list", "/list?p=0&size=1", "/search?keyword=test", "/detail/" + token.UUID} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code)
		require.NotContains(t, w.Body.String(), token.Key, path)
		require.NotContains(t, w.Body.String(), `"key"`, path)
		require.Contains(t, w.Body.String(), token.UUID, path)
		require.Contains(t, w.Body.String(), user.UUID, path)
		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, true, body["success"])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/owner/"+token.UUID, nil))
	require.Contains(t, w.Body.String(), token.Key, "authorized owner reveal remains available")
}
