package controller

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TotpCode string `json:"totp_code,omitempty"`
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
	lg := gmw.GetLogger(c)
	turnstileToken := c.Query("turnstile")
	middleware.RedactTurnstileTokenFromURL(c)

	var loginRequest LoginRequest
	err := json.NewDecoder(c.Request.Body).Decode(&loginRequest)
	if err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}
	username := loginRequest.Username
	password := loginRequest.Password
	if username == "" || password == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}

	// If this username has had a recent failed login and Turnstile is enabled, require verification.
	turnstileRequired := config.TurnstileCheckEnabled && middleware.HasLoginFailure(username)
	if turnstileRequired {
		if err := middleware.VerifyTurnstileToken(turnstileToken, c.ClientIP()); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
				"data": gin.H{
					"turnstile_required": true,
				},
			})
			return
		}
	}

	user := model.User{
		Username: username,
		Password: password,
	}
	err = user.ValidateAndFill()
	if err != nil {
		// Record failed attempt so next login for this username requires Turnstile.
		middleware.RecordLoginFailure(username)
		resp := gin.H{
			"message": err.Error(),
			"success": false,
		}
		if config.TurnstileCheckEnabled {
			resp["data"] = gin.H{
				"turnstile_required": true,
			}
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// Enforce PasswordLoginEnabled: when the admin disables password login,
	// only root users may still authenticate with username/password (so a
	// site operator can recover access if the SSO/IdP is unreachable).
	// All other roles must use a third-party method such as OIDC. The check
	// runs after ValidateAndFill so we never reveal account existence to
	// callers that supplied wrong credentials.
	if !config.PasswordLoginEnabled && user.Role < model.RoleRootUser {
		// Global logger: no request identity is bound at this point in the login
		// flow, so the resolved account must be named explicitly.
		lg.Debug("password login rejected: feature disabled for non-root user",
			append(user.Ref().Zap(), zap.Int("role", user.Role))...)
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("The administrator has disabled password login. Please use a third-party authentication method (e.g. OIDC) to log in.")))
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
				},
			})
			return
		}

		// Check rate limit for TOTP verification during login
		if !middleware.CheckTotpRateLimit(c, user.Id) {
			helper.RespondErrorWithStatus(c, http.StatusTooManyRequests, errors.New("Too many TOTP verification attempts. Please wait before trying again."))
			return
		}

		// Verify TOTP code
		if !verifyTotpCode(ctx, user.Id, user.TotpSecret, loginRequest.TotpCode) {
			helper.RespondError(c, errkind.UnauthorizedErr(errors.New("Invalid TOTP code")))
			return
		}
	}

	// Successful login — clear any failed login records for this username.
	middleware.ClearLoginFailure(username)
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

	// Id is intentionally omitted: ToResponse() emits only the external UUID, so
	// setting the internal integer id here would be dead (it never reaches the
	// wire). The boundary DTO makes that explicit instead of relying on an
	// ambient marshaler to drop it.
	cleanUser := model.User{
		UUID:        user.UUID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Status:      user.Status,
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "",
		"success": true,
		"data":    cleanUser.ToResponse(),
	})
}

func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	err := session.Save()
	if err != nil {
		helper.RespondError(c, err)
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
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("The administrator has turned off new user registration")))
		return
	}
	if !config.PasswordRegisterEnabled {
		helper.RespondError(c, errkind.ForbiddenErr(errors.New("The administrator has turned off registration via password. Please use the form of third-party account verification to register")))
		return
	}
	var req dto.UserRegisterRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidParameterMessage)))
		return
	}
	if err := common.Validate.Struct(&req); err != nil {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New(invalidInputMessage)))
		return
	}
	if config.EmailVerificationEnabled {
		if req.Email == "" || req.VerificationCode == "" {
			helper.RespondError(c, errkind.InvalidRequestErr(errors.New("The administrator has turned on email verification, please enter the email address and verification code")))
			return
		}
		if !common.VerifyCodeWithKey(req.Email, req.VerificationCode, common.EmailVerificationPurpose) {
			helper.RespondError(c, errkind.UnauthorizedErr(errors.New("Verification code error or expired")))
			return
		}
	}
	affCode := req.AffCode // this code is the inviter's code, not the user's own code
	inviterId, _ := model.GetUserIdByAffCode(affCode)
	cleanUser := model.User{
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.Username,
		InviterId:   inviterId,
	}
	if config.EmailVerificationEnabled {
		cleanUser.Email = req.Email
	}
	if model.IsUsernameAlreadyTaken(cleanUser.Username) {
		respondRegisterUsernameTaken(c)
		return
	}
	if err := cleanUser.Insert(ctx, inviterId); err != nil {
		if isRegisterUsernameTakenError(err) {
			respondRegisterUsernameTaken(c)
			return
		}
		helper.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// respondRegisterUsernameTaken returns the public duplicate-username registration response while preserving the legacy HTTP 200 envelope.
func respondRegisterUsernameTaken(c *gin.Context) {
	RespondUsernameAlreadyExists(c)
}

// isRegisterUsernameTakenError reports whether an insert error came from the username unique constraint.
func isRegisterUsernameTakenError(err error) bool {
	return IsUsernameAlreadyTakenError(err)
}
