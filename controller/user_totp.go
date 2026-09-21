package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
)

type TotpSetupRequest struct {
	TotpCode string `json:"totp_code"`
}

type TotpSetupResponse struct {
	Secret string `json:"secret"`
	QRCode string `json:"qr_code"`
}

// SetupTotp generates a new TOTP secret and QR code for the user
//
// Note ([H0llyW00dzZ]): This fixes double-encoding issues where config system name when we put space on it for example "One API" it literally break the encoding
// as I don't have repo/fork [github.com/Laisky/go-utils/v6/crypto] so I modified here and it default use sha1
//
// [github.com/Laisky/go-utils/v6/crypto]: https://github.com/Laisky/go-utils
// [H0llyW00dzZ]: https://github.com/H0llyW00dzZ
func SetupTotp(c *gin.Context) {
	userID := c.GetInt(ctxkey.Id)
	user, err := model.GetUserById(userID, true)
	if err != nil {
		helper.RespondError(c, errors.Wrapf(err, "get user %d", userID))
		return
	}
	if user.Metadata.PasswordLocked {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("MFA enrollment is locked by administrator")))
		return
	}
	// Generate a new secret
	secret := gcrypto.Base32Secret([]byte(random.GetRandomString(20)))

	// Create TOTP instance
	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
		AccountName:  user.Username,
		IssuerName:   config.SystemName,
	})
	if err != nil {
		helper.RespondError(c, errors.New("Failed to generate TOTP: "+err.Error()))
		return
	}

	// Store temporary secret in session
	session := sessions.Default(c)
	session.Set("temp_totp_secret", secret)
	session.Save()

	// Generate QR code URI from library
	originalURI := totp.URI()

	// Rebuild the URI with proper encoding to fix double-encoding issues
	// The library's URI() may double-encode spaces in system name
	// Parse and reconstruct: otpauth://totp/Issuer:AccountName?secret=SECRET&issuer=Issuer
	if _, err = url.Parse(originalURI); err != nil {
		// Fallback: build URI manually if parsing fails
		label := fmt.Sprintf("%s:%s", url.PathEscape(config.SystemName), url.PathEscape(user.Username))
		qrCodeURI := fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s",
			label,
			secret,
			url.PathEscape(config.SystemName),
		)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": TotpSetupResponse{
				Secret: secret,
				QRCode: qrCodeURI,
			},
		})
		return
	}

	// Rebuild with proper encoding
	label := fmt.Sprintf("%s:%s", url.PathEscape(config.SystemName), url.PathEscape(user.Username))
	qrCodeURI := fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=%s",
		label,
		secret,
		url.PathEscape(config.SystemName),
	)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": TotpSetupResponse{
			Secret: secret,
			QRCode: qrCodeURI,
		},
	})
}

// ConfirmTotp verifies the TOTP code and enables TOTP for the user
func ConfirmTotp(c *gin.Context) {
	ctx := gmw.Ctx(c)
	userId := c.GetInt(ctxkey.Id)

	// Check rate limit for TOTP verification
	if !middleware.CheckTotpRateLimit(c, userId) {
		helper.RespondErrorWithStatus(c, http.StatusTooManyRequests, errors.New("Too many TOTP verification attempts. Please wait before trying again."))
		return
	}

	var req TotpSetupRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	if req.TotpCode == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("TOTP code is required")))
		return
	}

	user, err := model.GetUserById(userId, true)
	if err != nil {
		helper.RespondError(c, errors.Wrapf(err, "get user %d", userId))
		return
	}
	if user.Metadata.PasswordLocked {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("MFA enrollment is locked by administrator")))
		return
	}

	// Get the temporary secret from session or generate error
	session := sessions.Default(c)
	tempSecret := session.Get("temp_totp_secret")
	if tempSecret == nil {
		helper.RespondError(c, errkind.UnauthorizedErr(errors.New("No TOTP setup session found. Please start setup again.")))
		return
	}

	secret := tempSecret.(string)

	// Verify the TOTP code
	if !verifyTotpCode(ctx, user.Id, secret, req.TotpCode) {
		helper.RespondError(c, errkind.UnauthorizedErr(errors.New("Invalid TOTP code")))
		return
	}

	// Save the secret to user
	user.TotpSecret = secret
	err = user.Update(false)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	// Clear the temporary secret from session
	session.Delete("temp_totp_secret")
	session.Save()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "TOTP has been successfully enabled",
	})
}

// DisableTotp disables TOTP for the user
func DisableTotp(c *gin.Context) {
	ctx := gmw.Ctx(c)
	userId := c.GetInt(ctxkey.Id)

	// Check rate limit for TOTP verification
	if !middleware.CheckTotpRateLimit(c, userId) {
		helper.RespondErrorWithStatus(c, http.StatusTooManyRequests, errors.New("Too many TOTP verification attempts. Please wait before trying again."))
		return
	}

	var req TotpSetupRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	user, err := model.GetUserById(userId, true)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	if user.TotpSecret == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("TOTP is not enabled for this user")))
		return
	}

	// Verify the TOTP code before disabling
	if !verifyTotpCode(ctx, user.Id, user.TotpSecret, req.TotpCode) {
		helper.RespondError(c, errkind.UnauthorizedErr(errors.New("Invalid TOTP code")))
		return
	}

	// Clear the TOTP secret
	err = user.ClearTotpSecret()
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "TOTP has been successfully disabled",
	})
}

// verifyTotpCode verifies a TOTP code against a secret with rate limiting and replay protection
func verifyTotpCode(ctx context.Context, uid int, secret, code string) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	lg := gmw.GetLogger(ctx)
	if lg == nil {
		lg = logger.Logger
	}
	if code == "" || secret == "" {
		return false
	}

	// Check if this TOTP code has been used recently (replay protection)
	if common.IsTotpCodeUsed(ctx, uid, code) {
		// ctx may be a bare background context here (TOTP is also verified off the
		// request goroutine), so resolve the account explicitly. This branch only
		// fires on a detected replay, never on the normal login path.
		lg.Warn("TOTP code replay attempt detected", model.LookupUserRef(ctx, uid).Zap()...)
		return false
	}

	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{
		Base32Secret: secret,
	})
	if err != nil {
		return false
	}

	// Verify the code
	verified := totp.Key() == code
	if !verified {
		return false
	}

	// Mark the code as used to prevent replay attacks
	err = common.MarkTotpCodeAsUsed(ctx, uid, code)
	if err != nil {
		lg.Error("Failed to mark TOTP code as used", zap.Error(err))
		// Don't fail the verification if we can't mark it as used
		// This ensures the system remains functional even if Redis/cache fails
	}

	return true
}

// GetTotpStatus returns whether TOTP is enabled for the current user
func GetTotpStatus(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	user, err := model.GetUserById(userId, true)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"totp_enabled": user.TotpSecret != "",
		},
	})
}

// AdminGetUserTotpStatus reports whether TOTP is enabled for the referenced user.
//
// The strict-out user DTO never carries totp_secret, so admin UIs need this
// endpoint to decide whether to offer the "disable TOTP" action.
//
// Parameters:
//   - c: gin context; path param "id" is the target user's UUID.
//
// Return values:
//   - JSON {"success": true, "data": {"totp_enabled": bool}} or an error response.
func AdminGetUserTotpStatus(c *gin.Context) {
	targetUserRef := c.Param("id")
	if targetUserRef == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	userId, err := resolveUserRef(targetUserRef)
	if err != nil {
		helper.RespondError(c, errkind.Mark(errors.New("Invalid user ID"), errkind.Of(err)))
		return
	}

	user, err := model.GetUserById(userId, true)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	myRole := c.GetInt(ctxkey.Role)
	if myRole <= user.Role && myRole != model.RoleRootUser {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("No permission to view user with the same or higher permission level")))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"totp_enabled": user.TotpSecret != "",
		},
	})
}

// AdminDisableUserTotp allows admins to disable TOTP for any user
func AdminDisableUserTotp(c *gin.Context) {
	ctx := gmw.Ctx(c)
	targetUserId := c.Param("id")
	if targetUserId == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	userId, err := resolveUserRef(targetUserId)
	if err != nil {
		// Inherit the resolver's attribution: only the malformed-reference and
		// not-found sentinels are client-caused. A lookup failure (database or
		// cache outage) stays Unknown so it keeps reaching the ERROR branch.
		helper.RespondError(c, errkind.Mark(errors.New("Invalid user ID"), errkind.Of(err)))
		return
	}

	// Get the target user
	user, err := model.GetUserById(userId, true)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	// Check if admin has permission to modify this user
	myRole := c.GetInt(ctxkey.Role)
	if myRole <= user.Role && myRole != model.RoleRootUser {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("No permission to modify user with the same or higher permission level")))
		return
	}

	// Check if TOTP is already disabled
	if user.TotpSecret == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("TOTP is not enabled for this user")))
		return
	}

	// Clear the TOTP secret
	err = user.ClearTotpSecret()
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	// Log the admin action
	adminUserId := c.GetInt(ctxkey.Id)
	note := fmt.Sprintf("%s target_username=%s", adminActorNote(adminUserId), user.Username)
	model.RecordManageLog(ctx, user.Id, "totp_enabled", true, false, note)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "TOTP has been successfully disabled for the user",
	})
}
