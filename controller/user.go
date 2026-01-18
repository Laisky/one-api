package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/blacklist"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/common/utils"
	"github.com/songquanpeng/one-api/dto"
	"github.com/songquanpeng/one-api/middleware"
	"github.com/songquanpeng/one-api/model"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TotpCode string `json:"totp_code,omitempty"`
}
type TotpSetupRequest struct {
	TotpCode string `json:"totp_code"`
}

type TotpSetupResponse struct {
	Secret string `json:"secret"`
	QRCode string `json:"qr_code"`
}

// rawFieldPresent reports whether a field key exists in the decoded JSON payload.
func rawFieldPresent(raw map[string]json.RawMessage, key string) bool {
	_, ok := raw[key]
	return ok
}

// jsonRawIsNull returns true when the raw JSON value is an explicit null literal.
func jsonRawIsNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func Login(c *gin.Context) {
	ctx := gmw.Ctx(c)
	var loginRequest LoginRequest
	err := json.NewDecoder(c.Request.Body).Decode(&loginRequest)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": invalidParameterMessage,
			"success": false,
		})
		return
	}
	username := loginRequest.Username
	password := loginRequest.Password
	if username == "" || password == "" {
		c.JSON(http.StatusOK, gin.H{
			"message": invalidParameterMessage,
			"success": false,
		})
		return
	}
	user := model.User{
		Username: username,
		Password: password,
	}
	err = user.ValidateAndFill()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}

	// Check if TOTP is enabled for this user
	if user.TotpSecret != "" {
		// TOTP is enabled, check if code is provided
		if loginRequest.TotpCode == "" {
			// Return special response indicating TOTP is required
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "totp_required",
				"data": gin.H{
					"totp_required": true,
					"user_id":       user.Id,
				},
			})
			return
		}

		// Check rate limit for TOTP verification during login
		if !middleware.CheckTotpRateLimit(c, user.Id) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "Too many TOTP verification attempts. Please wait before trying again.",
			})
			return
		}

		// Verify TOTP code
		if !verifyTotpCode(ctx, user.Id, user.TotpSecret, loginRequest.TotpCode) {
			c.JSON(http.StatusOK, gin.H{
				"message": "Invalid TOTP code",
				"success": false,
			})
			return
		}
	}

	SetupLogin(&user, c)
}

// setup session & cookies and then return user info
func SetupLogin(user *model.User, c *gin.Context) {
	// BUG: 如果用户发送了一段不合法的 session cookie，因为 gorilla 对无法识别的 session 会默认返回 nil，
	// 导致 session.Set 中会出现 panic
	//
	//   2025/04/16 01:20:29 [Recovery] 2025/04/16 - 01:20:29 panic recovered:
	//   runtime error: invalid memory address or nil pointer dereference
	//   /opt/go1.24.0/src/runtime/panic.go:262 (0x44b77d)
	//   	panicmem: panic(memoryError)
	//   /opt/go1.24.0/src/runtime/signal_unix.go:925 (0x48b764)
	//   	sigpanic: panicmem()
	//   /home/laisky/go/pkg/mod/github.com/gin-contrib/sessions@v1.0.3/sessions.go:88 (0x1601112)
	//   	(*session).Set: s.Session().Values[key] = val
	//   /home/laisky/repo/laisky/one-api/controller/user.go:70 (0x28145a7)
	//   	SetupLogin: session.Set("id", user.Id)
	//
	// BUG: https://github.com/gin-contrib/sessions/issues/287
	// github.com/gin-contrib/sessions 不要使用 v1.0.3
	session := sessions.Default(c)
	session.Set("id", user.Id)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	session.Set("status", user.Status)
	err := session.Save()
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "unable to save login session information"))
		return
	}

	// set auth header
	// c.Set("id", user.Id)
	// GenerateAccessToken(c)
	// c.Header("Authorization", user.AccessToken)

	cleanUser := model.User{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Status:      user.Status,
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
		"data":    cleanUser,
	})
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	err := session.Save()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": err.Error(),
			"success": false,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
	})
}

func Register(c *gin.Context) {
	ctx := gmw.Ctx(c)
	if !config.RegisterEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "The administrator has turned off new user registration",
			"success": false,
		})
		return
	}
	if !config.PasswordRegisterEnabled {
		c.JSON(http.StatusOK, gin.H{
			"message": "The administrator has turned off registration via password. Please use the form of third-party account verification to register",
			"success": false,
		})
		return
	}
	var user model.User
	err := json.NewDecoder(c.Request.Body).Decode(&user)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}
	if err := common.Validate.Struct(&user); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidInputMessage,
		})
		return
	}
	if config.EmailVerificationEnabled {
		if user.Email == "" || user.VerificationCode == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "The administrator has turned on email verification, please enter the email address and verification code",
			})
			return
		}
		if !common.VerifyCodeWithKey(user.Email, user.VerificationCode, common.EmailVerificationPurpose) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Verification code error or expired",
			})
			return
		}
	}
	affCode := user.AffCode // this code is the inviter's code, not the user's own code
	inviterId, _ := model.GetUserIdByAffCode(affCode)
	cleanUser := model.User{
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.Username,
		InviterId:   inviterId,
	}
	if config.EmailVerificationEnabled {
		cleanUser.Email = user.Email
	}
	if err := cleanUser.Insert(ctx, inviterId); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func GetAllUsers(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}

	// Get page size from query parameter, default to config value

	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	order := c.DefaultQuery("order", "")
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	users, err := model.GetAllUsers(p*size, size, order, sortBy, sortOrder)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Get total count for pagination
	totalCount, err := model.GetUserCount()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    users,
		"total":   totalCount,
	})
}

func SearchUsers(c *gin.Context) {
	keyword := c.Query("keyword")
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	users, err := model.SearchUsers(keyword, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    users,
	})
}

func GetUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user, err := model.GetUserById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	myRole := c.GetInt(ctxkey.Role)
	if myRole <= user.Role && myRole != model.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No permission to get information of users at the same level or higher",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user,
	})
}

// GetUserDashboard returns per-day per-model usage statistics and quota info.
// Date Range Semantics:
//
//	The API accepts `from_date` and `to_date` in YYYY-MM-DD format (UTC) and
//	interprets them as an inclusive range of whole days. Internally this is
//	converted into a half-open Unix second interval: [from_date 00:00:00 UTC, to_date+1 00:00:00 UTC).
//	This guarantees that the entire final day is included without relying on
//	second-based inclusivity or adding 24h-1s hacks, eliminating off-by-one
//	errors and DST complications.
//	Maximum range: regular users 7 days, root users 365 days.
func GetUserDashboard(c *gin.Context) {
	id := c.GetInt(ctxkey.Id)
	role := c.GetInt(ctxkey.Role)
	now := time.Now()

	// Parse date range parameters
	fromDateStr := c.Query("from_date")
	toDateStr := c.Query("to_date")

	// We will use half-open interval: [startTs, endTsExclusive)
	// to avoid off-by-one second issues and ensure full-day coverage.
	var startTs, endTsExclusive int64

	if fromDateStr != "" && toDateStr != "" {
		maxDays := 7
		if role == model.RoleRootUser {
			maxDays = 365
		}
		s, e, err := utils.NormalizeDateRange(fromDateStr, toDateStr, maxDays)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error(), "data": nil})
			return
		}
		startTs = s
		endTsExclusive = e
	} else {
		// Default last 7 days including today: [today-6, today]
		today := now.UTC().Truncate(24 * time.Hour)
		startTs = today.AddDate(0, 0, -6).Unix()
		endTsExclusive = today.Add(24 * time.Hour).Unix()
	}

	// Check if user wants to view specific user's data (root users only)
	targetUserId := id // Default to current user
	userIdParam := c.Query("user_id")

	if userIdParam != "" {
		// Only root users can view other users' data or site-wide data
		if role != model.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "No permission to view other users' dashboard data",
				"data":    nil,
			})
			return
		}

		if userIdParam == "all" {
			targetUserId = 0 // 0 means site-wide statistics
		} else {
			var err error
			targetUserId, err = strconv.Atoi(userIdParam)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Invalid user_id parameter",
					"data":    nil,
				})
				return
			}
		}
	} else if role == model.RoleRootUser {
		// For root users, default to site-wide statistics
		targetUserId = 0
	}

	// Get log statistics
	// Using half-open interval [startTs, endTsExclusive)
	dashboards, err := model.SearchLogsByDayAndModel(targetUserId, int(startTs), int(endTsExclusive))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to get dashboard data: " + err.Error(),
			"data":    nil,
		})
		return
	}

	userStats, err := model.SearchLogsByDayAndUser(targetUserId, int(startTs), int(endTsExclusive))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to get user usage data: " + err.Error(),
			"data":    nil,
		})
		return
	}

	tokenStats, err := model.SearchLogsByDayAndToken(targetUserId, int(startTs), int(endTsExclusive))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to get token usage data: " + err.Error(),
			"data":    nil,
		})
		return
	}

	// Get quota and status information
	var totalQuota, usedQuota int64
	var status string

	if targetUserId == 0 {
		// Site-wide statistics for admin/root users
		totalQuota, usedQuota, status, err = model.GetSiteWideQuotaStats()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to get site-wide quota stats: " + err.Error(),
				"data":    nil,
			})
			return
		}
	} else {
		// Individual user statistics
		user, err := model.GetUserById(targetUserId, false)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to get user data: " + err.Error(),
				"data":    nil,
			})
			return
		}
		totalQuota = user.Quota
		usedQuota = user.UsedQuota
		switch user.Status {
		case model.UserStatusEnabled:
			status = "Active"
		case model.UserStatusDisabled:
			status = "Disabled"
		case model.UserStatusDeleted:
			status = "Deleted"
		default:
			status = "Unknown"
		}
	}

	// Create response with both log data and quota/status info
	response := gin.H{
		"logs":        dashboards,
		"user_logs":   userStats,
		"token_logs":  tokenStats,
		"total_quota": totalQuota,
		"used_quota":  usedQuota,
		"status":      status,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

func GetDashboardUsers(c *gin.Context) {
	role := c.GetInt(ctxkey.Role)

	// Only root users can access this endpoint
	if role != model.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No permission to access user list",
			"data":    nil,
		})
		return
	}

	// Get all users with basic info (id, username, display_name)
	users, err := model.GetAllUsers(0, 1000, "", "", "") // Get up to 1000 users
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to get user list: " + err.Error(),
			"data":    nil,
		})
		return
	}

	// Create simplified user list for dropdown
	type UserOption struct {
		Id          int    `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
	}

	var userOptions []UserOption
	// Add "All Users" option first
	userOptions = append(userOptions, UserOption{
		Id:          0,
		Username:    "all",
		DisplayName: "All Users (Site-wide)",
	})

	// Add individual users
	for _, user := range users {
		userOptions = append(userOptions, UserOption{
			Id:          user.Id,
			Username:    user.Username,
			DisplayName: user.DisplayName,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    userOptions,
	})
}

func GenerateAccessToken(c *gin.Context) {
	id := c.GetInt(ctxkey.Id)
	user, err := model.GetUserById(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user.AccessToken = random.GetUUID()

	if model.DB.Where("access_token = ?", user.AccessToken).First(user).RowsAffected != 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Please try again, the system-generated UUID is actually duplicated!",
		})
		return
	}

	if err := user.Update(false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
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
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if user.AffCode == "" {
		user.AffCode = random.GetRandomString(4)
		if err := user.Update(false); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
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
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "missing token context",
		})
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
		"id":           user.Id,
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
		"id":               token.Id,
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
		"uid":                    user.Id,
		"username":               user.Username,
		"token_id":               token.Id,
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
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    user,
	})
}

func UpdateUser(c *gin.Context) {
	ctx := gmw.Ctx(c)
	adminUserID := c.GetInt(ctxkey.Id)
	body, err := common.GetRequestBody(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	var payload dto.UserAdminUpdatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	if payload.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	originUser, err := model.GetUserById(payload.Id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	myRole := c.GetInt(ctxkey.Role)
	if myRole <= originUser.Role && myRole != model.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No permission to update user information with the same permission level or higher permission level",
		})
		return
	}

	updates := make(map[string]any)
	var (
		quotaUpdated  bool
		newQuota      int64
		statusChanged bool
		newStatus     int
	)

	if rawFieldPresent(raw, "username") {
		if jsonRawIsNull(raw["username"]) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Username cannot be null"})
			return
		}
		var username string
		if err := json.Unmarshal(raw["username"], &username); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": invalidParameterMessage,
			})
			return
		}
		username = strings.TrimSpace(username)
		if username == "" {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Username cannot be empty"})
			return
		}
		if utf8.RuneCountInString(username) < 3 || utf8.RuneCountInString(username) > 30 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Username must be between 3 and 30 characters"})
			return
		}
		if myRole <= originUser.Role && myRole != model.RoleRootUser && username != originUser.Username {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "No permission to rename this user"})
			return
		}
		updates["username"] = username
	}

	if rawFieldPresent(raw, "display_name") {
		if jsonRawIsNull(raw["display_name"]) {
			// nil => no change
		} else {
			var displayName string
			if err := json.Unmarshal(raw["display_name"], &displayName); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": invalidParameterMessage,
				})
				return
			}
			displayName = strings.TrimSpace(displayName)
			if utf8.RuneCountInString(displayName) > 20 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Display name cannot exceed 20 characters"})
				return
			}
			updates["display_name"] = displayName
		}
	}

	if rawFieldPresent(raw, "email") {
		if jsonRawIsNull(raw["email"]) {
			// nil => no change
		} else {
			var email string
			if err := json.Unmarshal(raw["email"], &email); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": invalidParameterMessage,
				})
				return
			}
			email = strings.TrimSpace(email)
			if email != "" {
				if utf8.RuneCountInString(email) > 50 {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "Email cannot exceed 50 characters"})
					return
				}
				if err := common.Validate.Var(email, "email"); err != nil {
					c.JSON(http.StatusOK, gin.H{"success": false, "message": "Valid email is required"})
					return
				}
			}
			updates["email"] = email
		}
	}

	if rawFieldPresent(raw, "group") {
		if jsonRawIsNull(raw["group"]) {
			// nil => no change
		} else {
			var group string
			if err := json.Unmarshal(raw["group"], &group); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": invalidParameterMessage,
				})
				return
			}
			group = strings.TrimSpace(group)
			if group == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Group cannot be empty"})
				return
			}
			if utf8.RuneCountInString(group) > 32 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Group cannot exceed 32 characters"})
				return
			}
			updates["group"] = group
		}

		if rawFieldPresent(raw, "mcp_tool_blacklist") {
			if jsonRawIsNull(raw["mcp_tool_blacklist"]) {
				updates["mcp_tool_blacklist"] = nil
			} else {
				var blacklist []string
				if err := json.Unmarshal(raw["mcp_tool_blacklist"], &blacklist); err != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": invalidParameterMessage,
					})
					return
				}
				updates["mcp_tool_blacklist"] = blacklist
			}
		}
	}

	if rawFieldPresent(raw, "quota") {
		if jsonRawIsNull(raw["quota"]) {
			// nil => no change
		} else {
			if payload.Quota == nil {
				var quotaValue int64
				if err := json.Unmarshal(raw["quota"], &quotaValue); err != nil {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": invalidParameterMessage,
					})
					return
				}
				payload.Quota = &quotaValue
			}
			if *payload.Quota < 0 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Quota must be non-negative"})
				return
			}
			newQuota = *payload.Quota
			updates["quota"] = newQuota
			quotaUpdated = true
		}
	}

	if rawFieldPresent(raw, "password") {
		if jsonRawIsNull(raw["password"]) {
			// nil => no change
		} else {
			var password string
			if err := json.Unmarshal(raw["password"], &password); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": invalidParameterMessage,
				})
				return
			}
			password = strings.TrimSpace(password)
			if password == "" {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Password cannot be empty"})
				return
			}
			if utf8.RuneCountInString(password) < 8 || utf8.RuneCountInString(password) > 20 {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Password length must be between 8 and 20 characters"})
				return
			}
			hashed, hashErr := common.Password2Hash(password)
			if hashErr != nil {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": hashErr.Error()})
				return
			}
			updates["password"] = hashed
		}
	}

	if rawFieldPresent(raw, "role") {
		if jsonRawIsNull(raw["role"]) {
			// nil => no change
		} else {
			var roleValue int
			if payload.Role != nil {
				roleValue = *payload.Role
			} else if err := json.Unmarshal(raw["role"], &roleValue); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": invalidParameterMessage,
				})
				return
			}
			if myRole <= roleValue && myRole != model.RoleRootUser {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "No permission to promote other users to a permission level greater than or equal to your own",
				})
				return
			}
			updates["role"] = roleValue
		}
	}

	if rawFieldPresent(raw, "status") {
		if jsonRawIsNull(raw["status"]) {
			// nil => no change
		} else {
			var statusValue int
			if payload.Status != nil {
				statusValue = *payload.Status
			} else if err := json.Unmarshal(raw["status"], &statusValue); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": invalidParameterMessage,
				})
				return
			}
			switch statusValue {
			case model.UserStatusEnabled, model.UserStatusDisabled, model.UserStatusDeleted:
			default:
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "Invalid status provided"})
				return
			}
			updates["status"] = statusValue
			statusChanged = originUser.Status != statusValue
			newStatus = statusValue
		}
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
		})
		return
	}

	if err := model.DB.Model(&model.User{}).Where("id = ?", payload.Id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": errors.Wrapf(err, "failed to update user: id=%d", payload.Id).Error(),
		})
		return
	}

	if statusChanged {
		switch newStatus {
		case model.UserStatusDisabled:
			blacklist.BanUser(payload.Id)
		case model.UserStatusEnabled:
			blacklist.UnbanUser(payload.Id)
		}
	}

	if quotaUpdated && originUser.Quota != newQuota {
		note := fmt.Sprintf("admin_id=%d", adminUserID)
		model.RecordManageLog(ctx, originUser.Id, "quota", common.LogQuota(originUser.Quota), common.LogQuota(newQuota), note)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateSelf(c *gin.Context) {
	var user model.User
	err := json.NewDecoder(c.Request.Body).Decode(&user)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}
	if user.Password == "" {
		user.Password = "$I_LOVE_U" // make Validator happy :)
	}
	if err := common.Validate.Struct(&user); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Input is illegal " + err.Error(),
		})
		return
	}

	// Disallow empty username/display name
	if strings.TrimSpace(user.Username) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Username cannot be empty"})
		return
	}
	if strings.TrimSpace(user.DisplayName) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Display name cannot be empty"})
		return
	}

	cleanUser := model.User{
		Id:          c.GetInt(ctxkey.Id),
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
	}
	if user.Password == "$I_LOVE_U" {
		user.Password = "" // rollback to what it should be
		cleanUser.Password = ""
	}
	updatePassword := user.Password != ""
	if err := cleanUser.Update(updatePassword); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	originUser, err := model.GetUserById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	myRole := c.GetInt("role")
	if myRole <= originUser.Role {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No permission to delete users with the same permission level or higher permission level",
		})
		return
	}
	err = model.DeleteUserById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
		})
		return
	}
}

func DeleteSelf(c *gin.Context) {
	id := c.GetInt("id")
	user, _ := model.GetUserById(id, false)

	if user.Role == model.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Cannot delete super administrator account",
		})
		return
	}

	err := model.DeleteUserById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func CreateUser(c *gin.Context) {
	ctx := gmw.Ctx(c)
	var user model.User
	err := json.NewDecoder(c.Request.Body).Decode(&user)
	if err != nil || user.Username == "" || user.Password == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}
	if err := common.Validate.Struct(&user); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidInputMessage,
		})
		return
	}
	// Disallow empty username/display name
	if strings.TrimSpace(user.Username) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Username cannot be empty"})
		return
	}
	if user.DisplayName != "" && strings.TrimSpace(user.DisplayName) == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Display name cannot be empty if provided"})
		return
	}
	if user.DisplayName == "" {
		user.DisplayName = user.Username
	}
	myRole := c.GetInt("role")
	if user.Role >= myRole {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Unable to create users with permissions greater than or equal to your own",
		})
		return
	}
	// Even for admin users, we cannot fully trust them!
	cleanUser := model.User{
		Username:    user.Username,
		Password:    user.Password,
		DisplayName: user.DisplayName,
	}
	if err := cleanUser.Insert(ctx, 0); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

type ManageRequest struct {
	Username string `json:"username"`
	Action   string `json:"action"`
}

// ManageUser Only admin user can do this
func ManageUser(c *gin.Context) {
	var req ManageRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}
	user := model.User{
		Username: req.Username,
	}
	// Fill attributes
	model.DB.Where(&user).First(&user)
	if user.Id == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "User does not exist",
		})
		return
	}
	myRole := c.GetInt("role")
	if myRole <= user.Role && myRole != model.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No permission to update user information with the same permission level or higher permission level",
		})
		return
	}
	switch req.Action {
	case "disable":
		user.Status = model.UserStatusDisabled
		if user.Role == model.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Unable to disable super administrator user",
			})
			return
		}
	case "enable":
		user.Status = model.UserStatusEnabled
	case "delete":
		if user.Role == model.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Unable to delete super administrator user",
			})
			return
		}
		if err := user.Delete(); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "promote":
		if myRole != model.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Ordinary administrator users cannot promote other users to administrators",
			})
			return
		}
		if user.Role >= model.RoleAdminUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "The user is already an administrator",
			})
			return
		}
		user.Role = model.RoleAdminUser
	case "demote":
		if user.Role == model.RoleRootUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Unable to downgrade super administrator user",
			})
			return
		}
		if user.Role == model.RoleCommonUser {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "The user is already an ordinary user",
			})
			return
		}
		user.Role = model.RoleCommonUser
	}

	if err := user.Update(false); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	clearUser := model.User{
		Role:   user.Role,
		Status: user.Status,
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    clearUser,
	})
}

func EmailBind(c *gin.Context) {
	email := c.Query("email")
	code := c.Query("code")
	if !common.VerifyCodeWithKey(email, code, common.EmailVerificationPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Verification code error or expired",
		})
		return
	}
	id := c.GetInt("id")
	user := model.User{
		Id: id,
	}
	err := user.FillUserById()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	user.Email = email
	// no need to check if this email already taken, because we have used verification code to check it
	err = user.Update(false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
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
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	id := c.GetInt("id")
	quota, err := model.Redeem(ctx, req.Key, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    quota,
	})
}

type adminTopUpRequest struct {
	UserId int    `json:"user_id"`
	Quota  int    `json:"quota"`
	Remark string `json:"remark"`
}

func AdminTopUp(c *gin.Context) {
	ctx := gmw.Ctx(c)
	req := adminTopUpRequest{}
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = model.IncreaseUserQuota(ctx, req.UserId, int64(req.Quota))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if req.Remark == "" {
		req.Remark = fmt.Sprintf("Recharged via API %s", common.LogQuota(int64(req.Quota)))
	}
	model.RecordTopupLog(ctx, req.UserId, req.Remark, req.Quota)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
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
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
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
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to generate TOTP: " + err.Error(),
		})
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
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "Too many TOTP verification attempts. Please wait before trying again.",
		})
		return
	}

	var req TotpSetupRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	if req.TotpCode == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "TOTP code is required",
		})
		return
	}

	user, err := model.GetUserById(userId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Get the temporary secret from session or generate error
	session := sessions.Default(c)
	tempSecret := session.Get("temp_totp_secret")
	if tempSecret == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No TOTP setup session found. Please start setup again.",
		})
		return
	}

	secret := tempSecret.(string)

	// Verify the TOTP code
	if !verifyTotpCode(ctx, user.Id, secret, req.TotpCode) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid TOTP code",
		})
		return
	}

	// Save the secret to user
	user.TotpSecret = secret
	err = user.Update(false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
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
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "Too many TOTP verification attempts. Please wait before trying again.",
		})
		return
	}

	var req TotpSetupRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	user, err := model.GetUserById(userId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if user.TotpSecret == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "TOTP is not enabled for this user",
		})
		return
	}

	// Verify the TOTP code before disabling
	if !verifyTotpCode(ctx, user.Id, user.TotpSecret, req.TotpCode) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid TOTP code",
		})
		return
	}

	// Clear the TOTP secret
	err = user.ClearTotpSecret()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
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
		lg.Warn(fmt.Sprintf("TOTP code replay attempt detected for user %d", uid))
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
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
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
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	// Convert string ID to int
	userId, err := strconv.Atoi(targetUserId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid user ID",
		})
		return
	}

	// Get the target user
	user, err := model.GetUserById(userId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Check if admin has permission to modify this user
	myRole := c.GetInt(ctxkey.Role)
	if myRole <= user.Role && myRole != model.RoleRootUser {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "No permission to modify user with the same or higher permission level",
		})
		return
	}

	// Check if TOTP is already disabled
	if user.TotpSecret == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "TOTP is not enabled for this user",
		})
		return
	}

	// Clear the TOTP secret
	err = user.ClearTotpSecret()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Log the admin action
	adminUserId := c.GetInt(ctxkey.Id)
	note := fmt.Sprintf("admin_id=%d target_username=%s", adminUserId, user.Username)
	model.RecordManageLog(ctx, user.Id, "totp_enabled", true, false, note)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "TOTP has been successfully disabled for the user",
	})
}
