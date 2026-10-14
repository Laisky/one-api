package controller

import (
	"net/http"

	"github.com/Laisky/one-api/common/config"
	"github.com/gin-gonic/gin"
)

// GetBrowserConfig projects only the validated public runtime Sentry setting and never returns raw options, credentials, or release metadata.
func GetBrowserConfig(c *gin.Context) {
	config.OptionMapRWMutex.RLock()
	raw := config.OptionMap["BrowserSentryConfig"]
	config.OptionMapRWMutex.RUnlock()
	value, err := config.ParseBrowserSentryConfig(raw)
	if err != nil {
		value = nil
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.JSON(http.StatusOK, gin.H{"sentry": value})
}
