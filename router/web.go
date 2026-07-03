package router

import (
	"embed"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/controller"
	"github.com/Laisky/one-api/middleware"
)

const agentDiscoveryLinks = "<https://oneapi.laisky.com/llms.txt>; rel=\"describedby\"; type=\"text/plain\", " +
	"<https://oneapi.laisky.com/index.md>; rel=\"alternate\"; type=\"text/markdown\", " +
	"<https://oneapi.laisky.com/sitemap.xml>; rel=\"sitemap\"; type=\"application/xml\", " +
	"<https://oneapi.laisky.com/openapi.json>; rel=\"service-desc\"; type=\"application/vnd.oai.openapi+json\", " +
	"<https://oneapi.laisky.com/.well-known/api-catalog>; rel=\"api-catalog\"; type=\"application/linkset+json\", " +
	"<https://oneapi.laisky.com/.well-known/ai-catalog.json>; rel=\"describedby\"; type=\"application/json\", " +
	"<https://oneapi.laisky.com/.well-known/agent-card.json>; rel=\"describedby\"; type=\"application/json\", " +
	"<https://oneapi.laisky.com/.well-known/mcp/manifest.json>; rel=\"service-desc\"; type=\"application/json\""

// SetWebRouter registers static frontend assets and agent-readable discovery
// endpoints. Parameters: router is the Gin engine and buildFS contains the
// embedded frontend build. Return value: none; the function mutates router.
func SetWebRouter(router *gin.Engine, buildFS embed.FS) {
	indexPageData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/index.html", config.Theme))
	indexMarkdownData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/index.md", config.Theme))
	agentModeData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/agent-mode.md", config.Theme))
	apiCatalogData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/.well-known/api-catalog.json", config.Theme))
	aiCatalogData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/.well-known/ai-catalog.json", config.Theme))
	agentCardData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/.well-known/agent-card.json", config.Theme))
	agentSkillsIndexData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/.well-known/agent-skills/index.json", config.Theme))
	httpMessageSignaturesDirectoryData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/.well-known/http-message-signatures-directory", config.Theme))
	mcpManifestData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/.well-known/mcp/manifest.json", config.Theme))
	openAPIData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/openapi.json", config.Theme))
	openAPIMarkdownData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/openapi.json.md", config.Theme))
	oauthAuthorizationServerData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/.well-known/oauth-authorization-server", config.Theme))
	oauthProtectedResourceData, _ := buildFS.ReadFile(fmt.Sprintf("web/build/%s/.well-known/oauth-protected-resource", config.Theme))
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	router.Use(addAgentDiscoveryHeaders())

	router.GET("/", func(c *gin.Context) {
		if c.Query("mode") == "agent" && len(agentModeData) > 0 {
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/markdown; charset=utf-8", agentModeData)
			return
		}

		if wantsMarkdown(c) && len(indexMarkdownData) > 0 {
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "text/markdown; charset=utf-8", indexMarkdownData)
			return
		}

		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPageData)
	})
	router.GET("/.well-known/api-catalog", func(c *gin.Context) {
		if len(apiCatalogData) == 0 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, "application/linkset+json; charset=utf-8", apiCatalogData)
	})
	router.GET("/.well-known/api-catalog.json", func(c *gin.Context) {
		if len(apiCatalogData) == 0 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, "application/linkset+json; charset=utf-8", apiCatalogData)
	})
	router.GET("/.well-known/ai-catalog.json", servePreparedAgentData(aiCatalogData, "application/json; charset=utf-8"))
	router.GET("/.well-known/agent-card.json", servePreparedAgentData(agentCardData, "application/json; charset=utf-8"))
	router.GET(
		"/.well-known/agent-skills/index.json",
		servePreparedAgentData(agentSkillsIndexData, "application/json; charset=utf-8"),
	)
	router.GET(
		"/.well-known/http-message-signatures-directory",
		servePreparedAgentData(httpMessageSignaturesDirectoryData, "application/json; charset=utf-8"),
	)
	router.GET("/openapi.json", servePreparedAgentData(openAPIData, "application/vnd.oai.openapi+json; charset=utf-8"))
	router.GET("/swagger.json", servePreparedAgentData(openAPIData, "application/vnd.oai.openapi+json; charset=utf-8"))
	router.GET("/openapi.json.md", servePreparedAgentData(openAPIMarkdownData, "text/markdown; charset=utf-8"))
	router.GET(
		"/.well-known/mcp/manifest.json",
		servePreparedAgentData(mcpManifestData, "application/json; charset=utf-8"),
	)
	router.GET("/.well-known/mcp", servePreparedAgentData(mcpManifestData, "application/json; charset=utf-8"))
	router.GET(
		"/.well-known/oauth-authorization-server",
		servePreparedAgentData(oauthAuthorizationServerData, "application/json; charset=utf-8"),
	)
	router.GET(
		"/.well-known/oauth-protected-resource",
		servePreparedAgentData(oauthProtectedResourceData, "application/json; charset=utf-8"),
	)
	router.GET("/docs/llms.txt", serveFileFromBuild(buildFS, "docs/llms.txt", "text/plain; charset=utf-8"))
	router.GET("/api/llms.txt", serveFileFromBuild(buildFS, "api/llms.txt", "text/plain; charset=utf-8"))
	router.GET("/developers/llms.txt", serveFileFromBuild(buildFS, "developers/llms.txt", "text/plain; charset=utf-8"))
	router.GET("/docs", serveMarkdownFromBuild(buildFS, "docs.md"))
	router.GET("/developers", serveMarkdownFromBuild(buildFS, "developers.md"))
	router.GET("/api-reference", serveMarkdownFromBuild(buildFS, "api.md"))

	router.Use(static.Serve("/", common.EmbedFolder(buildFS, fmt.Sprintf("web/build/%s", config.Theme))))
	router.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPageData)
	})
}

// addAgentDiscoveryHeaders returns middleware that advertises machine-readable
// agent resources through RFC 8288 Link headers. Parameters: none. Return
// value: a Gin middleware function.
func addAgentDiscoveryHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Link", agentDiscoveryLinks)
		c.Header("Vary", "Accept, Accept-Encoding")
		c.Next()
	}
}

// serveMarkdownFromBuild returns a handler that serves a markdown document from
// the selected embedded frontend build. Parameters: buildFS is the embedded
// filesystem and filename is relative to the theme build root. Return value: a
// Gin handler that serves text/markdown or 404 when the file is absent.
func serveMarkdownFromBuild(buildFS embed.FS, filename string) gin.HandlerFunc {
	return serveFileFromBuild(buildFS, filename, "text/markdown; charset=utf-8")
}

// serveFileFromBuild returns a handler that serves a static document from the
// selected embedded frontend build. Parameters: buildFS is the embedded
// filesystem, filename is relative to the theme build root, and contentType is
// the HTTP Content-Type value. Return value: a Gin handler that serves bytes or
// 404 when the file is absent.
func serveFileFromBuild(buildFS embed.FS, filename string, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := buildFS.ReadFile(fmt.Sprintf("web/build/%s/%s", config.Theme, filename))
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, contentType, data)
	}
}

// servePreparedAgentData returns a handler that serves preloaded agent
// discovery bytes. Parameters: data is the response body and contentType is the
// HTTP Content-Type value. Return value: a Gin handler that serves the data or
// 404 when it was not embedded.
func servePreparedAgentData(data []byte, contentType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(data) == 0 {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		c.Header("Cache-Control", "max-age=604800")
		c.Data(http.StatusOK, contentType, data)
	}
}

// wantsMarkdown reports whether a request prefers markdown over HTML.
// Parameters: c carries the incoming request headers. Return value: true when
// the Accept header asks for text/markdown.
func wantsMarkdown(c *gin.Context) bool {
	return strings.Contains(strings.ToLower(c.GetHeader("Accept")), "text/markdown")
}
