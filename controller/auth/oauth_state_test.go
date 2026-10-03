package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// seedOAuthState stores the supplied state in the session and persists it to the response cookie.
func seedOAuthState(c *gin.Context, state string) {
	seedOAuthSessionValue(c, state)
}

// seedOAuthSessionValue stores the supplied state value in the session and persists it to the response cookie.
func seedOAuthSessionValue(c *gin.Context, state any) {
	session := sessions.Default(c)
	session.Set("oauth_state", state)
	if err := session.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// newOAuthTestRouter returns a minimal router with session middleware and the OIDC callback under test.
func newOAuthTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	store := cookie.NewStore([]byte("test-secret"))
	router.Use(sessions.Sessions("test-session", store))
	router.GET("/seed", func(c *gin.Context) {
		seedOAuthState(c, c.Query("state"))
	})
	router.GET("/seed-non-string", func(c *gin.Context) {
		seedOAuthSessionValue(c, 42)
	})
	router.GET("/api/oauth/oidc", OidcAuth)
	return router
}

// decodeJSONResponse parses a standard JSON response into a map for assertions.
func decodeJSONResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	return payload
}

// addCookies copies response cookies into the supplied request.
func addCookies(req *http.Request, recorder *httptest.ResponseRecorder) {
	for _, cookie := range recorder.Result().Cookies() {
		req.AddCookie(cookie)
	}
}

// TestOidcAuth_RejectsMissingSessionState verifies that the callback still rejects
// requests when the session-backed OAuth state is absent.
func TestOidcAuth_RejectsMissingSessionState(t *testing.T) {
	router := newOAuthTestRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/oidc?code=test-code&state=expected-state", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	payload := decodeJSONResponse(t, w)
	require.Equal(t, false, payload["success"])
	require.Equal(t, "state is empty or not same", payload["message"])
}

// TestOidcAuth_RejectsMismatchedSessionState verifies that different request
// and session state values fail before the OAuth code is exchanged.
func TestOidcAuth_RejectsMismatchedSessionState(t *testing.T) {
	router := newOAuthTestRouter()
	seedReq := httptest.NewRequest(http.MethodGet, "/seed?state=expected-state", nil)
	seedResp := httptest.NewRecorder()
	router.ServeHTTP(seedResp, seedReq)
	require.Equal(t, http.StatusOK, seedResp.Code)

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/oauth/oidc?code=test-code&state=wrong-state", nil)
	addCookies(callbackReq, seedResp)
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)

	require.Equal(t, http.StatusForbidden, callbackResp.Code)
	payload := decodeJSONResponse(t, callbackResp)
	require.Equal(t, false, payload["success"])
	require.Equal(t, "state is empty or not same", payload["message"])
}

// TestOidcAuth_RejectsNonStringSessionState verifies that malformed session
// state values fail closed instead of panicking during type assertion.
func TestOidcAuth_RejectsNonStringSessionState(t *testing.T) {
	router := newOAuthTestRouter()
	seedReq := httptest.NewRequest(http.MethodGet, "/seed-non-string", nil)
	seedResp := httptest.NewRecorder()
	router.ServeHTTP(seedResp, seedReq)
	require.Equal(t, http.StatusOK, seedResp.Code)

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/oauth/oidc?code=test-code&state=expected-state", nil)
	addCookies(callbackReq, seedResp)
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)

	require.Equal(t, http.StatusForbidden, callbackResp.Code)
	payload := decodeJSONResponse(t, callbackResp)
	require.Equal(t, false, payload["success"])
	require.Equal(t, "state is empty or not same", payload["message"])
}

// TestOidcAuth_AcceptsMatchingSessionState verifies that a valid session state passes
// the CSRF guard and reaches the feature-gate branch instead of failing with 403.
func TestOidcAuth_AcceptsMatchingSessionState(t *testing.T) {
	originalOidcEnabled := config.OidcEnabled
	t.Cleanup(func() {
		config.OidcEnabled = originalOidcEnabled
	})
	config.OidcEnabled = false

	router := newOAuthTestRouter()
	seedReq := httptest.NewRequest(http.MethodGet, "/seed?state=expected-state", nil)
	seedResp := httptest.NewRecorder()
	router.ServeHTTP(seedResp, seedReq)
	require.Equal(t, http.StatusOK, seedResp.Code)

	callbackReq := httptest.NewRequest(http.MethodGet, "/api/oauth/oidc?code=test-code&state=expected-state", nil)
	addCookies(callbackReq, seedResp)
	callbackResp := httptest.NewRecorder()
	router.ServeHTTP(callbackResp, callbackReq)

	require.Equal(t, http.StatusOK, callbackResp.Code)
	payload := decodeJSONResponse(t, callbackResp)
	require.Equal(t, false, payload["success"])
	require.Equal(t, "Administrator has not enabled OIDC Log in and Sign up", payload["message"])

	replayReq := httptest.NewRequest(http.MethodGet, "/api/oauth/oidc?code=test-code&state=expected-state", nil)
	addCookies(replayReq, callbackResp)
	replayResp := httptest.NewRecorder()
	router.ServeHTTP(replayResp, replayReq)

	require.Equal(t, http.StatusForbidden, replayResp.Code)
	replayPayload := decodeJSONResponse(t, replayResp)
	require.Equal(t, false, replayPayload["success"])
	require.Equal(t, "state is empty or not same", replayPayload["message"])
}
