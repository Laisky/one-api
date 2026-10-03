package controller

import (
	"github.com/Laisky/one-api/common/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSecurityAdminTokensDoNotDiscloseKeys verifies list, search and detail keep other users' credentials private.
func TestSecurityAdminTokensDoNotDiscloseKeys(t *testing.T) {
	cleanup, _, token := setupConsumeTokenTest(t)
	defer cleanup()
	r := gin.New()
	r.GET("/list", AdminGetAllTokens)
	r.GET("/search", AdminSearchTokens)
	r.GET("/detail/:id", AdminGetToken)
	r.GET("/owner/:id", func(c *gin.Context) { c.Set("id", token.UserId); GetToken(c) })
	for _, path := range []string{"/list", "/search?keyword=test", "/detail/" + token.UUID} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, 200, w.Code)
		require.NotContains(t, w.Body.String(), token.Key)
		require.Contains(t, w.Body.String(), token.UUID)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/owner/"+token.UUID, nil))
	require.True(t, strings.Contains(w.Body.String(), token.Key), "owner secret retrieval remains supported")
}

// TestSecurityDisabledRootPasswordLogin verifies the global disable setting applies to root.
func TestSecurityDisabledRootPasswordLogin(t *testing.T) {
	setupPasswordLoginDisabledTest(t)
	createLoginUser(t, "root-boundary", "fixture-password", 100)
	config.PasswordLoginEnabled = false
	w := postLogin(t, newLoginRouter(), "root-boundary", "fixture-password")
	require.Contains(t, w.Body.String(), `"success":false`)
	require.Empty(t, w.Header().Values("Set-Cookie"))
}
