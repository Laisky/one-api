package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/message"
	"github.com/Laisky/one-api/model"
)

// GetStatus returns application metadata and feature toggles for the public status endpoint.
func GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"version":                     common.Version,
			"start_time":                  common.StartTime,
			"email_verification":          config.EmailVerificationEnabled,
			"github_oauth":                config.GitHubOAuthEnabled,
			"github_client_id":            config.GitHubClientId,
			"lark_client_id":              config.LarkClientId,
			"system_name":                 config.SystemName,
			"logo":                        config.Logo,
			"footer_html":                 config.Footer,
			"wechat_qrcode":               config.WeChatAccountQRCodeImageURL,
			"wechat_login":                config.WeChatAuthEnabled,
			"server_address":              config.ServerAddress,
			"turnstile_check":             config.TurnstileCheckEnabled,
			"turnstile_site_key":          config.TurnstileSiteKey,
			"top_up_link":                 config.TopUpLink,
			"chat_link":                   config.ChatLink,
			"quota_per_unit":              config.QuotaPerUnit,
			"display_in_currency":         config.DisplayInCurrencyEnabled,
			"oidc":                        config.OidcEnabled,
			"oidc_client_id":              config.OidcClientId,
			"oidc_well_known":             config.OidcWellKnown,
			"oidc_authorization_endpoint": config.OidcAuthorizationEndpoint,
			"oidc_token_endpoint":         config.OidcTokenEndpoint,
			"oidc_userinfo_endpoint":      config.OidcUserinfoEndpoint,
		},
	})
}

// GetNotice returns the configured notice content for the UI.
func GetNotice(c *gin.Context) {
	config.OptionMapRWMutex.RLock()
	defer config.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    config.OptionMap["Notice"],
	})
}

// GetAbout returns the configured about content for the UI.
func GetAbout(c *gin.Context) {
	config.OptionMapRWMutex.RLock()
	defer config.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    config.OptionMap["About"],
	})
}

// GetHomePageContent returns the configured homepage content block.
func GetHomePageContent(c *gin.Context) {
	config.OptionMapRWMutex.RLock()
	defer config.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    config.OptionMap["HomePageContent"],
	})
}

// SendEmailVerification issues a verification code to the provided email address.
func SendEmailVerification(c *gin.Context) {
	email := c.Query("email")
	if err := common.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	// Simulate processing time to mitigate timing attacks
	time.Sleep(time.Second)

	// Always return a uniform success response to mitigate user enumeration
	// and timing attacks. The actual verification process and email sending
	// are performed asynchronously.
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "If the email is valid and not already registered, you will receive a verification code shortly.",
	})

	lg := gmw.GetLogger(gmw.BackgroundCtx(c))
	go func() {
		// Perform domain whitelist check and email occupancy check in the
		// background to prevent timing attacks.
		if config.EmailDomainRestrictionEnabled {
			allowed := false
			for _, domain := range config.EmailDomainWhitelist {
				if strings.HasSuffix(email, "@"+domain) {
					allowed = true
					break
				}
			}
			if !allowed {
				return
			}
		}

		if model.IsEmailAlreadyTaken(email) {
			return
		}

		code := common.GenerateVerificationCode(6)
		common.RegisterVerificationCodeWithKey(email, code, common.EmailVerificationPurpose)
		subject := fmt.Sprintf("%s Email Verification", config.SystemName)
		content := message.EmailTemplate(
			subject,
			fmt.Sprintf(`
			<p>Hello!</p>
			<p>You are verifying your email for %s.</p>
			<p>Your verification code is:</p>
			<p style="font-size: 24px; font-weight: bold; color: #333; background-color: #f8f8f8; padding: 10px; text-align: center; border-radius: 4px;">%s</p>
			<p style="color: #666;">The verification code is valid for %d minutes. If you did not request this, please ignore.</p>
		`, config.SystemName, code, common.VerificationValidMinutes),
		)

		err := message.SendEmail(subject, email, content)
		if err != nil {
			lg.Error("failed to send email verification", zap.Error(err))
		}
	}()
}

// SendPasswordResetEmail sends a password reset link to the supplied email address when registered.
func SendPasswordResetEmail(c *gin.Context) {
	email := c.Query("email")
	if err := common.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}

	// Always return a uniform success response to mitigate user enumeration
	// and timing attacks. The actual email sending is performed asynchronously.
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "If the email is registered, you will receive a password reset link shortly.",
	})

	lg := gmw.GetLogger(c)
	go func() {
		// To prevent timing attacks, we perform the email existence check
		// and the actual email sending in a background goroutine.
		if !model.IsEmailAlreadyTaken(email) {
			lg.Debug("password reset requested for unregistered email")
			return
		}

		code := common.GenerateVerificationCode(0)
		common.RegisterVerificationCodeWithKey(email, code, common.PasswordResetPurpose)
		link := fmt.Sprintf("%s/user/reset?email=%s&token=%s", config.ServerAddress, email, code)
		subject := fmt.Sprintf("%s Password Reset", config.SystemName)
		content := message.EmailTemplate(
			subject,
			fmt.Sprintf(`
			<p>Hello!</p>
			<p>You are resetting your password for %s.</p>
			<p>Please click the button below to reset your password:</p>
			<p style="text-align: center; margin: 30px 0;">
				<a href="%s" style="background-color: #007bff; color: white; padding: 12px 24px; text-decoration: none; border-radius: 4px; display: inline-block;">Reset Password</a>
			</p>
			<p style="color: #666;">If the button doesn't work, please copy the following link and paste it into your browser:</p>
			<p style="background-color: #f8f8f8; padding: 10px; border-radius: 4px; word-break: break-all;">%s</p>
			<p style="color: #666;">The reset link is valid for %d minutes. If you didn't request this, please ignore.</p>
		`, config.SystemName, link, link, common.VerificationValidMinutes),
		)

		if err := message.SendEmail(subject, email, content); err != nil {
			lg.Error("failed to send password reset email", zap.Error(err))
		} else {
			lg.Debug("password reset email sent successfully")
		}
	}()
}

type PasswordResetRequest struct {
	Email    string `json:"email"`
	Token    string `json:"token"`
	Password string `json:"password"`
}

// ResetPassword validates the reset token and assigns a new random password to the account.
func ResetPassword(c *gin.Context) {
	lg := gmw.GetLogger(c)
	var req PasswordResetRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if err != nil {
		lg.Debug("failed to decode password reset request", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}
	if req.Email == "" || req.Token == "" {
		lg.Debug("password reset request missing email or token",
			zap.Bool("email_empty", req.Email == ""),
			zap.Bool("token_empty", req.Token == ""))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": invalidParameterMessage,
		})
		return
	}
	if !common.VerifyCodeWithKey(req.Email, req.Token, common.PasswordResetPurpose) {
		lg.Debug("password reset token verification failed",
			zap.String("email", req.Email))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Reset link is illegal or expired",
		})
		return
	}

	var lockedCheckUser model.User
	if err := model.DB.Where("email = ?", req.Email).First(&lockedCheckUser).Error; err != nil {
		lg.Error("failed to look up user for password reset",
			zap.String("email", req.Email),
			zap.Error(errors.Wrapf(err, "look up user by email %s", req.Email)))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if lockedCheckUser.Metadata.PasswordLocked {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Password is locked by administrator",
		})
		return
	}

	// Use user-provided password if present; otherwise generate a random one
	// for backward compatibility with legacy frontends.
	password := req.Password
	if password == "" {
		password = common.GenerateVerificationCode(12)
	}

	err = model.ResetUserPasswordByEmail(req.Email, password)
	if err != nil {
		lg.Error("failed to reset password", zap.String("email", req.Email), zap.Error(err))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	common.DeleteKey(req.Email, common.PasswordResetPurpose)
	lg.Info("password reset successful", zap.String("email", req.Email),
		zap.Bool("user_provided_password", req.Password != ""))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    password,
	})
}

// GetChannelStatus returns a paginated view of channel health and recent test metrics.
func GetChannelStatus(c *gin.Context) {
	// Parse pagination parameters
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}

	// Get page size from query parameter, default to 6 as requested
	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = 6 // Default to 6 channels per page as requested
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	// Get channels with pagination for monitoring
	channels, err := model.GetAllChannels(p*size, size, "all", "", "")
	if err != nil {
		// Return a generic "Internal Server Error" as per best practices,
		// since database errors are rare and typically indicate an internal issue.
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Get total count for pagination
	totalCount, err := model.GetChannelCount()
	if err != nil {
		// Return a generic "Internal Server Error" as per best practices,
		// since database errors are rare and typically indicate an internal issue.
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Format channels for monitoring
	var channelStatuses []gin.H
	for _, channel := range channels {
		var status string
		var enabled bool

		switch channel.Status {
		case 1: // ChannelStatusEnabled
			status = "enabled"
			enabled = true
		case 2: // ChannelStatusManuallyDisabled
			status = "manually_disabled"
			enabled = false
		case 3: // ChannelStatusAutoDisabled
			status = "auto_disabled"
			enabled = false
		default: // ChannelStatusUnknown
			status = "unknown"
			enabled = false
		}

		channelStatus := gin.H{
			"name":    channel.Name,
			"status":  status,
			"enabled": enabled,
			"response": gin.H{
				"response_time_ms": channel.ResponseTime,
				"test_time":        channel.TestTime,
				"created_time":     channel.CreatedTime,
			},
		}
		channelStatuses = append(channelStatuses, channelStatus)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    channelStatuses,
		"total":   totalCount,
	})
}
