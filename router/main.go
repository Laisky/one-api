package router

import (
	"embed"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

func SetRouter(router *gin.Engine, buildFS embed.FS) {
	SetApiRouter(router)
	SetDashboardRouter(router)
	SetRelayRouter(router)
	frontendBaseUrl := config.FrontendBaseURL
	if config.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		logger.Logger.Info("FRONTEND_BASE_URL is ignored on master node")
	}
	if frontendBaseUrl == "" {
		SetWebRouter(router, buildFS)
	} else {
		trimmedBase := strings.TrimRight(frontendBaseUrl, "/")
		router.NoRoute(func(c *gin.Context) {
			// Reconstruct the target URL via url.ParseRequestURI so a hand-crafted
			// RequestURI (e.g. an absolute form or a non-origin form) cannot escape
			// the configured frontend origin. Standard browser requests use origin-form
			// (always starting with `/`); anything else here is suspect.
			parsed, err := url.ParseRequestURI(c.Request.RequestURI)
			if err != nil || !strings.HasPrefix(parsed.Path, "/") {
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
			c.Redirect(http.StatusMovedPermanently, trimmedBase+parsed.RequestURI())
		})
	}
}
