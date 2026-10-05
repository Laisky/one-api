package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// oauthTotpTestSecret is the base32 TOTP secret of the pending-login fixture.
const oauthTotpTestSecret = "JBSWY3DPEHPK3PXP"

// oauthTotpHarness wires CompleteOAuthLogin and OAuthTotpLogin to a session
// router and tracks the session cookie like a browser.
type oauthTotpHarness struct {
	router *gin.Engine
	db     *gorm.DB
	cookie *http.Cookie
	userID int
}

// newOAuthTotpHarness creates a TOTP-enabled account with a process-unique id
// and a router exposing the challenge, completion and session-inspection routes.
func newOAuthTotpHarness(t *testing.T) *oauthTotpHarness {
	t.Helper()
	db, cleanup := setupTestEnvironment(t)
	t.Cleanup(cleanup)
	oldRate := config.RateLimitDisabled
	config.RateLimitDisabled = true
	t.Cleanup(func() { config.RateLimitDisabled = oldRate })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	userID := int(time.Now().UTC().UnixNano()%1_000_000_000) + 3_000_000
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "pending-totp", Password: "unused", TotpSecret: oauthTotpTestSecret, Role: model.RoleCommonUser, Status: model.UserStatusEnabled, AffCode: "PTOTP", AccessToken: "pending-totp-access"}).Error)
	h := &oauthTotpHarness{router: setupTestRouter(), db: db, userID: userID}
	h.router.POST("/challenge", func(c *gin.Context) {
		user := model.User{Id: userID}
		require.NoError(t, user.FillUserById())
		CompleteOAuthLogin(&user, c)
	})
	h.router.POST("/forge", func(c *gin.Context) {
		session := sessions.Default(c)
		switch c.Query("kind") {
		case "string_id":
			session.Set(oauthTotpPendingUserKey, "1")
			session.Set(oauthTotpPendingExpiresKey, oauthTotpNow().Add(time.Minute).Unix())
		case "far_future":
			session.Set(oauthTotpPendingUserKey, userID)
			session.Set(oauthTotpPendingExpiresKey, oauthTotpNow().Add(time.Hour).Unix())
		}
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	h.router.POST("/api/oauth/totp", OAuthTotpLogin)
	h.router.GET("/whoami", func(c *gin.Context) {
		session := sessions.Default(c)
		c.JSON(http.StatusOK, gin.H{
			"id":      session.Get("id"),
			"pending": session.Get(oauthTotpPendingUserKey) != nil,
		})
	})
	return h
}

// do sends one request with the tracked cookie and records any new cookie.
func (h *oauthTotpHarness) do(method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if h.cookie != nil {
		request.AddCookie(h.cookie)
	}
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, request)
	for _, cookie := range w.Result().Cookies() {
		h.cookie = cookie
	}
	return w
}

// whoami returns the session inspection body.
func (h *oauthTotpHarness) whoami() string {
	return h.do(http.MethodGet, "/whoami", "").Body.String()
}

// code returns the current TOTP code for the fixture secret.
func (h *oauthTotpHarness) code(t *testing.T) string {
	t.Helper()
	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{Base32Secret: oauthTotpTestSecret})
	require.NoError(t, err)
	return totp.Key()
}

// TestOAuthTotpPendingLoginLifecycle covers every exit of the pending OAuth
// login: success clears the marker and authenticates; expiry, a disabled
// second factor, a disabled account, malformed or over-long markers and a bad
// body never authenticate.
func TestOAuthTotpPendingLoginLifecycle(t *testing.T) {
	t.Run("success_clears_marker", func(t *testing.T) {
		h := newOAuthTotpHarness(t)
		challenge := h.do(http.MethodPost, "/challenge", "")
		require.Contains(t, challenge.Body.String(), `"totp_required":true`)
		require.Contains(t, h.whoami(), `"pending":true`)
		require.Contains(t, h.whoami(), `"id":null`)

		ok := h.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+h.code(t)+`"}`)
		require.Contains(t, ok.Body.String(), `"success":true`)
		require.Contains(t, ok.Body.String(), "pending-totp")
		require.Contains(t, h.whoami(), `"pending":false`)
		require.NotContains(t, h.whoami(), `"id":null`)
	})

	t.Run("expired", func(t *testing.T) {
		h := newOAuthTotpHarness(t)
		h.do(http.MethodPost, "/challenge", "")
		oldNow := oauthTotpNow
		t.Cleanup(func() { oauthTotpNow = oldNow })
		oauthTotpNow = func() time.Time { return oldNow().Add(oauthTotpPendingTTL + time.Second) }
		expired := h.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+h.code(t)+`"}`)
		require.Contains(t, expired.Body.String(), `"totp_expired":true`)
		oauthTotpNow = oldNow
		require.Contains(t, h.whoami(), `"pending":false`, "expired marker was not cleared")
		require.Contains(t, h.whoami(), `"id":null`)
	})

	t.Run("totp_disabled_after_challenge", func(t *testing.T) {
		h := newOAuthTotpHarness(t)
		h.do(http.MethodPost, "/challenge", "")
		require.NoError(t, h.db.Model(&model.User{}).Where("id = ?", h.userID).Update("totp_secret", "").Error)
		w := h.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+h.code(t)+`"}`)
		require.Contains(t, w.Body.String(), `"totp_expired":true`)
		require.Contains(t, h.whoami(), `"id":null`)
	})

	t.Run("account_disabled_after_challenge", func(t *testing.T) {
		h := newOAuthTotpHarness(t)
		h.do(http.MethodPost, "/challenge", "")
		require.NoError(t, h.db.Model(&model.User{}).Where("id = ?", h.userID).Update("status", model.UserStatusDisabled).Error)
		w := h.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+h.code(t)+`"}`)
		require.Contains(t, w.Body.String(), "User has been banned")
		require.Contains(t, h.whoami(), `"pending":false`)
		require.Contains(t, h.whoami(), `"id":null`)
	})

	for _, kind := range []string{"string_id", "far_future"} {
		t.Run("forged_"+kind, func(t *testing.T) {
			h := newOAuthTotpHarness(t)
			h.do(http.MethodPost, "/forge?kind="+kind, "")
			w := h.do(http.MethodPost, "/api/oauth/totp", `{"totp_code":"`+h.code(t)+`"}`)
			require.Contains(t, w.Body.String(), `"totp_expired":true`)
			require.Contains(t, h.whoami(), `"id":null`)
		})
	}

	t.Run("bad_body", func(t *testing.T) {
		h := newOAuthTotpHarness(t)
		h.do(http.MethodPost, "/challenge", "")
		w := h.do(http.MethodPost, "/api/oauth/totp", `not-json`)
		require.Contains(t, w.Body.String(), `"success":false`)
		require.Contains(t, h.whoami(), `"pending":true`)
		require.Contains(t, h.whoami(), `"id":null`)
	})
}
