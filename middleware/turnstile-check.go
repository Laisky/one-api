package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/Laisky/errors/v2"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
)

type turnstileCheckResponse struct {
	Success bool `json:"success"`
}

// VerifyTurnstileToken validates a Turnstile token against the Cloudflare API.
// Returns nil on success, or an error describing the failure.
func VerifyTurnstileToken(token, clientIP string) error {
	if token == "" {
		return errors.New("Turnstile token is empty")
	}
	rawRes, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", url.Values{
		"secret":   {config.TurnstileSecretKey},
		"response": {token},
		"remoteip": {clientIP},
	})
	if err != nil {
		return errors.Wrap(err, "turnstile check request failed")
	}
	defer rawRes.Body.Close()
	var res turnstileCheckResponse
	if err = json.NewDecoder(rawRes.Body).Decode(&res); err != nil {
		return errors.Wrap(err, "turnstile response decode failed")
	}
	if !res.Success {
		return errors.New("turnstile verification failed")
	}
	return nil
}

func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.TurnstileCheckEnabled {
			session := sessions.Default(c)
			turnstileChecked := session.Get("turnstile")
			if turnstileChecked != nil {
				c.Next()
				return
			}
			if err := VerifyTurnstileToken(c.Query("turnstile"), c.ClientIP()); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": err.Error(),
				})
				c.Abort()
				return
			}
			session.Set("turnstile", true)
			err := session.Save()
			if err != nil {
				AbortWithError(c, http.StatusOK, errors.Wrap(err, "unable to save turnstile session information"))
				return
			}
		}
		c.Next()
	}
}
