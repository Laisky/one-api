package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/blacklist"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
)

const (
	// oauthTotpPendingUserKey stores the account id of a pending OAuth login.
	oauthTotpPendingUserKey = "oauth_totp_pending_user_id"
	// oauthTotpPendingExpiresKey stores the UTC Unix expiry of that login.
	oauthTotpPendingExpiresKey = "oauth_totp_pending_expires_at"
	// oauthTotpPendingTTL bounds how long a proven OAuth identity may wait for
	// its TOTP code before the provider login must be repeated.
	oauthTotpPendingTTL = 5 * time.Minute
	// maxOAuthTotpBodyBytes bounds the completion request body.
	maxOAuthTotpBodyBytes = 4 * 1024
	// oauthTotpExpiredMessage is returned when no valid pending login exists.
	oauthTotpExpiredMessage = "Two-factor sign-in has expired or was not started. Please sign in again."
)

// oauthTotpNow returns the current UTC time; tests replace it to cross the TTL.
var oauthTotpNow = func() time.Time { return time.Now().UTC() }

// oauthTotpRequest is the body of POST /api/oauth/totp.
type oauthTotpRequest struct {
	TotpCode string `json:"totp_code"`
}

// CompleteOAuthLogin finishes a login whose identity an OAuth or WeChat
// provider has proven for the enabled account user. Accounts without TOTP are
// signed in immediately through SetupLogin. For accounts with TOTP it does not
// create an authenticated session: it stores a short-lived pending marker
// (account id and UTC expiry) in the signed session and answers the same
// totp_required contract as password login, so the second factor must be
// presented to OAuthTotpLogin. It returns no value and always writes a response.
func CompleteOAuthLogin(user *model.User, c *gin.Context) {
	if user.TotpSecret == "" {
		SetupLogin(user, c)
		return
	}
	session := sessions.Default(c)
	session.Set(oauthTotpPendingUserKey, user.Id)
	session.Set(oauthTotpPendingExpiresKey, oauthTotpNow().Add(oauthTotpPendingTTL).Unix())
	if err := session.Save(); err != nil {
		helper.RespondError(c, errors.Wrap(err, "save pending OAuth TOTP login"))
		return
	}
	// No request identity is bound before login, so name the account explicitly.
	gmw.GetLogger(c).Debug("oauth login awaiting TOTP verification", user.Ref().Zap()...)
	respondTotpRequired(c)
}

// OAuthTotpLogin completes a pending OAuth or WeChat login with the account's
// TOTP code from the JSON body {"totp_code": "..."}. It requires an unexpired
// pending marker created by CompleteOAuthLogin, re-reads the account, applies
// the per-account TOTP rate limit and the shared single-use verifier, and only
// then creates the dashboard session through SetupLogin. A wrong code keeps the
// marker so the user can retry; a missing, expired or no-longer-applicable
// marker is cleared. It returns no value and always writes a response.
func OAuthTotpLogin(c *gin.Context) {
	ctx := gmw.Ctx(c)
	var req oauthTotpRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxOAuthTotpBodyBytes)
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}
	userID, ok := pendingOAuthTotpUser(c)
	if !ok {
		respondOAuthTotpExpired(c)
		return
	}
	if !middleware.CheckTotpRateLimit(c, userID) {
		helper.RespondErrorWithStatus(c, http.StatusTooManyRequests, errkind.RateLimitedErr(errors.New("Too many TOTP verification attempts. Please wait before trying again.")))
		return
	}
	user := model.User{Id: userID}
	if err := user.FillUserById(); err != nil {
		if errkind.Of(err) == errkind.NotFound {
			clearPendingOAuthTotp(c)
			respondOAuthTotpExpired(c)
			return
		}
		helper.RespondError(c, err)
		return
	}
	if user.Status != model.UserStatusEnabled || blacklist.IsUserBanned(user.Id) {
		clearPendingOAuthTotp(c)
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("User has been banned")))
		return
	}
	if user.TotpSecret == "" {
		// TOTP was disabled after the challenge; require a fresh provider login
		// instead of silently upgrading a stale pending marker.
		clearPendingOAuthTotp(c)
		respondOAuthTotpExpired(c)
		return
	}
	if !verifyTotpCode(ctx, user.Id, user.TotpSecret, req.TotpCode) {
		helper.RespondError(c, errkind.UnauthorizedErr(errors.New("Invalid TOTP code")))
		return
	}
	SetupLogin(&user, c)
}

// pendingOAuthTotpUser returns the account id of an unexpired pending OAuth
// login stored in c's session and true. A malformed or expired marker is
// cleared and reported as false, as is a missing one.
func pendingOAuthTotpUser(c *gin.Context) (int, bool) {
	session := sessions.Default(c)
	rawID, rawExpiry := session.Get(oauthTotpPendingUserKey), session.Get(oauthTotpPendingExpiresKey)
	userID, validID := rawID.(int)
	expiresAt, validExpiry := rawExpiry.(int64)
	now := oauthTotpNow()
	if validID && validExpiry && userID > 0 && now.Unix() < expiresAt && expiresAt <= now.Add(oauthTotpPendingTTL).Unix() {
		return userID, true
	}
	if rawID != nil || rawExpiry != nil {
		clearPendingOAuthTotp(c)
	}
	return 0, false
}

// clearPendingOAuthTotp removes the pending OAuth login marker from c's
// session and saves it. A save failure is logged, because the caller is
// already answering the request.
func clearPendingOAuthTotp(c *gin.Context) {
	session := sessions.Default(c)
	session.Delete(oauthTotpPendingUserKey)
	session.Delete(oauthTotpPendingExpiresKey)
	if err := session.Save(); err != nil {
		gmw.GetLogger(c).Warn("failed to clear pending OAuth TOTP login", zap.Error(err))
	}
}

// respondTotpRequired writes the totp_required contract shared by password
// and OAuth logins.
func respondTotpRequired(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "totp_required",
		"data": gin.H{
			"totp_required": true,
		},
	})
}

// respondOAuthTotpExpired tells the client that no pending OAuth login can be
// completed and that the provider login must be restarted.
func respondOAuthTotpExpired(c *gin.Context) {
	gmw.GetLogger(c).Debug("oauth TOTP completion without a valid pending login")
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": oauthTotpExpiredMessage,
		"data": gin.H{
			"totp_expired": true,
		},
	})
}
