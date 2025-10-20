package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/i18n"
	"github.com/songquanpeng/one-api/common/message"
	"github.com/songquanpeng/one-api/model"
)

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

func GetNotice(c *gin.Context) {
	config.OptionMapRWMutex.RLock()
	defer config.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    config.OptionMap["Notice"],
	})
}

func GetAbout(c *gin.Context) {
	config.OptionMapRWMutex.RLock()
	defer config.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    config.OptionMap["About"],
	})
}

func GetHomePageContent(c *gin.Context) {
	config.OptionMapRWMutex.RLock()
	defer config.OptionMapRWMutex.RUnlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    config.OptionMap["HomePageContent"],
	})
}

func SendEmailVerification(c *gin.Context) {
	email := c.Query("email")
	if err := common.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": i18n.Translate(c, "invalid_parameter"),
		})
		return
	}
	if config.EmailDomainRestrictionEnabled {
		allowed := false
		for _, domain := range config.EmailDomainWhitelist {
			if strings.HasSuffix(email, "@"+domain) {
				allowed = true
				break
			}
		}
		if !allowed {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Administrator has enabled email domain whitelist, your email domain is not in the whitelist",
			})
			return
		}
	}
	if model.IsEmailAlreadyTaken(email) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Email address is occupied",
		})
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

func SendPasswordResetEmail(c *gin.Context) {
	email := c.Query("email")
	if err := common.Validate.Var(email, "required,email"); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": i18n.Translate(c, "invalid_parameter"),
		})
		return
	}
	if !model.IsEmailAlreadyTaken(email) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "The email address is not registered",
		})
		return
	}
	code := common.GenerateVerificationCode(0)
	common.RegisterVerificationCodeWithKey(email, code, common.PasswordResetPurpose)
	link := fmt.Sprintf("%s/user/reset?email=%s&token=%s", config.ServerAddress, email, code)
	subject := fmt.Sprintf("%s 密码重置", config.SystemName)
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
	err := message.SendEmail(subject, email, content)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("%s%s", i18n.Translate(c, "send_email_failed"), err.Error()),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

type PasswordResetRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

func ResetPassword(c *gin.Context) {
	var req PasswordResetRequest
	err := json.NewDecoder(c.Request.Body).Decode(&req)
	if req.Email == "" || req.Token == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": i18n.Translate(c, "invalid_parameter"),
		})
		return
	}
	if !common.VerifyCodeWithKey(req.Email, req.Token, common.PasswordResetPurpose) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Reset link is illegal or expired",
		})
		return
	}
	password := common.GenerateVerificationCode(12)
	err = model.ResetUserPasswordByEmail(req.Email, password)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	common.DeleteKey(req.Email, common.PasswordResetPurpose)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    password,
	})
}

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
