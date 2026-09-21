package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
)

func GenerateAccessToken(c *gin.Context) {
	id := c.GetInt(ctxkey.Id)
	user, err := model.GetUserById(id, true)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	user.AccessToken = random.GetUUID()

	if model.DB.Where("access_token = ?", user.AccessToken).First(user).RowsAffected != 0 {
		helper.RespondError(c, errors.New("Please try again, the system-generated UUID is actually duplicated!"))
		return
	}

	if err := user.Update(false); err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.AccessToken,
	})
}

func GetAffCode(c *gin.Context) {
	id := c.GetInt(ctxkey.Id)
	user, err := model.GetUserById(id, true)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if user.AffCode == "" {
		user.AffCode = random.GetRandomString(4)
		if err := user.Update(false); err != nil {
			helper.RespondError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.AffCode,
	})
}

// GetSelfByToken returns the authenticated user and token metadata for API key calls.
func GetSelfByToken(c *gin.Context) {
	userID := c.GetInt(ctxkey.Id)
	tokenID := c.GetInt(ctxkey.TokenId)
	if userID == 0 || tokenID == 0 {
		helper.RespondErrorWithStatus(c, http.StatusBadRequest, errors.New("missing token context"))
		return
	}

	user, err := model.GetUserById(userID, false)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	token, err := model.GetTokenByIds(tokenID, userID)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	userData := gin.H{
		"uuid":         user.UUID,
		"username":     user.Username,
		"display_name": user.DisplayName,
		"role":         user.Role,
		"status":       user.Status,
		"group":        user.Group,
		"quota":        user.Quota,
		"used_quota":   user.UsedQuota,
		"created_at":   user.CreatedAt,
		"updated_at":   user.UpdatedAt,
	}

	var models any
	if token.Models != nil {
		if trimmed := strings.TrimSpace(*token.Models); trimmed != "" {
			models = trimmed
		}
	}

	var subnet any
	if token.Subnet != nil {
		if trimmed := strings.TrimSpace(*token.Subnet); trimmed != "" {
			subnet = trimmed
		}
	}

	tokenData := gin.H{
		"uuid":             token.UUID,
		"user_uuid":        token.UserUUID,
		"name":             token.Name,
		"status":           token.Status,
		"remain_quota":     token.RemainQuota,
		"used_quota":       token.UsedQuota,
		"unlimited_quota":  token.UnlimitedQuota,
		"expired_time":     token.ExpiredTime,
		"accessed_time":    token.AccessedTime,
		"created_time":     token.CreatedTime,
		"created_at":       token.CreatedAt,
		"updated_at":       token.UpdatedAt,
		"models":           models,
		"subnet":           subnet,
		"available_models": c.GetString(ctxkey.AvailableModels),
	}

	response := gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"user":  userData,
			"token": tokenData,
		},
		"user_uuid":              user.UUID,
		"username":               user.Username,
		"token_uuid":             token.UUID,
		"token_name":             token.Name,
		"token_status":           token.Status,
		"token_used_quota":       token.UsedQuota,
		"token_remain_quota":     token.RemainQuota,
		"token_unlimited_quota":  token.UnlimitedQuota,
		"token_created_time":     token.CreatedTime,
		"token_updated_at":       token.UpdatedAt,
		"token_accessed_time":    token.AccessedTime,
		"token_expired_time":     token.ExpiredTime,
		"token_available_models": tokenData["available_models"],
	}

	c.JSON(http.StatusOK, response)
}

func GetSelf(c *gin.Context) {
	id := c.GetInt(ctxkey.Id)
	user, err := model.GetUserById(id, false)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user.ToResponse(),
	})
}

func UpdateSelf(c *gin.Context) {
	lg := gmw.GetLogger(c)
	var user dto.UserSelfUpdateRequest
	if err := common.UnmarshalBodyReusable(c, &user); err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	// Inspect the raw payload so we can distinguish "field omitted" from
	// "field provided but empty". This matters for fields like display_name
	// that the user may legitimately want to clear.
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "get request body"))
		return
	}
	rawFields := make(map[string]json.RawMessage)
	if len(requestBody) > 0 {
		if err := json.Unmarshal(requestBody, &rawFields); err != nil {
			// Body is not valid JSON.
			helper.RespondError(c, errkind.InvalidRequestErr(errors.Wrap(err, "unmarshal raw user self payload")))
			return
		}
	}
	displayNameProvided := rawFieldPresent(rawFields, "display_name")

	// When frontend sends only a subset of fields (e.g. password-only update),
	// fill in missing username/display_name from the current user record so that
	// partial updates don't fail with "cannot be empty" errors.
	userId := c.GetInt(ctxkey.Id)
	currentUser, fetchErr := model.GetUserById(userId, false)
	if fetchErr != nil {
		helper.RespondError(c, fetchErr)
		return
	}
	// Username is the user's login identity (unique, max=30) and is treated as
	// required. Keep the silent-restore so partial payloads (e.g. password-only)
	// continue to work and never accidentally blank out the login key.
	if strings.TrimSpace(user.Username) == "" {
		user.Username = currentUser.Username
	}
	// DisplayName is optional; only fall back to the existing value when the
	// caller did NOT supply the field at all. An explicitly provided empty
	// string must clear the value.
	if !displayNameProvided {
		user.DisplayName = currentUser.DisplayName
	}

	if user.Password != "" && currentUser.Metadata.PasswordLocked {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("Password is locked by administrator")))
		return
	}

	if user.Password == "" {
		user.Password = "$I_LOVE_U" // make Validator happy :)
	}
	// Validate against a model.User (not the request DTO) so the go-playground
	// validator error message keeps the exact "User.<Field>" struct prefix the
	// client received before the request-DTO refactor. Only
	// Username/Password/DisplayName/Email carry validate tags, so the validation
	// result is identical while the error body stays byte-for-byte the same (T18).
	if err := common.Validate.Struct(&model.User{
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
		Email:       user.Email,
	}); err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("Input is illegal "+err.Error())))
		return
	}

	cleanUser := model.User{
		Id:          userId,
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
	}
	if user.Password == "$I_LOVE_U" {
		user.Password = "" // rollback to what it should be
		cleanUser.Password = ""
	}
	updatePassword := user.Password != ""
	if cleanUser.Username != currentUser.Username && model.IsUsernameAlreadyTaken(cleanUser.Username) {
		respondRegisterUsernameTaken(c)
		return
	}
	if err := cleanUser.Update(updatePassword); err != nil {
		if isRegisterUsernameTakenError(err) {
			respondRegisterUsernameTaken(c)
			return
		}
		helper.RespondError(c, err)
		return
	}

	// User.Update relies on GORM's Updates(struct), which skips zero-value
	// strings. To honor an explicit empty display_name we need a targeted
	// update that includes the column unconditionally. Avoid logging the
	// value itself (potential PII), only the user id.
	if displayNameProvided && strings.TrimSpace(user.DisplayName) == "" {
		if err := model.DB.Model(&model.User{}).Where("id = ?", userId).Update("display_name", "").Error; err != nil {
			helper.RespondError(c, errors.Wrapf(err, "clear display_name for user: id=%d", userId))
			return
		}
		// Global logger: carries no request identity, so name the account here.
		lg.Debug("user cleared display_name", currentUser.Ref().Zap()...)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteSelf(c *gin.Context) {
	id := c.GetInt("id")
	user, _ := model.GetUserById(id, false)

	if user.Role == model.RoleRootUser {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("Cannot delete super administrator account")))
		return
	}

	err := model.DeleteUserById(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func EmailBind(c *gin.Context) {
	email := c.Query("email")
	code := c.Query("code")
	if !common.VerifyCodeWithKey(email, code, common.EmailVerificationPurpose) {
		helper.RespondError(c, errkind.UnauthorizedErr(errors.New("Verification code error or expired")))
		return
	}
	id := c.GetInt("id")
	user := model.User{
		Id: id,
	}
	err := user.FillUserById()
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	user.Email = email
	// no need to check if this email already taken, because we have used verification code to check it
	err = user.Update(false)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if user.Role == model.RoleRootUser {
		config.RootUserEmail = email
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

type topUpRequest struct {
	Key string `json:"key"`
}

func TopUp(c *gin.Context) {
	ctx := gmw.Ctx(c)
	req := topUpRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		// Malformed request body: the caller sent JSON this endpoint cannot bind.
		helper.RespondError(c, errkind.InvalidRequestErr(err))
		return
	}
	id := c.GetInt("id")

	// Throttle enumeration/brute-force of redemption codes: an authenticated user
	// who has already burned through their failed-attempt budget is rejected
	// before any DB work, without leaking whether the submitted code is valid.
	if middleware.IsRedeemBlocked(c, id) {
		// The request-scoped logger already carries the caller's identity.
		gmw.GetLogger(c).Warn("redemption blocked: too many failed attempts",
			zap.String("client_ip", c.ClientIP()),
		)
		helper.RespondErrorWithStatus(c, http.StatusTooManyRequests,
			errors.New("too many failed redemption attempts, please try again later"))
		return
	}

	quota, err := model.Redeem(ctx, req.Key, id)
	if err != nil {
		// Record the failure against the user's budget and log who attempted the
		// redemption and what code they tried, so brute-force / enumeration of
		// redemption codes can be throttled and the offending account identified.
		// The attempted key is logged because a failed attempt means the code is
		// invalid or already used, so it has no residual value, and the pattern
		// of attempts is useful forensically. Successful redemptions are not
		// recorded, so a legitimate user is never throttled.
		middleware.RecordRedeemFailure(c, id)
		// The request-scoped logger already carries the caller's identity.
		gmw.GetLogger(c).Warn("redemption attempt failed",
			zap.String("client_ip", c.ClientIP()),
			zap.String("attempted_key", req.Key),
			zap.Error(err),
		)
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    quota,
	})
}
