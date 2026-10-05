package router

import (
	"bytes"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/Laisky/one-api/common/config"
	"github.com/gin-gonic/gin"
)

const discoveryTemplateOrigin = "https://oneapi.laisky.com"
const discoveryOriginContextKey = "oneapi.discovery.origin"

// configuredDiscoveryOrigin returns a validated administrator-configured HTTP origin,
// or an empty string when the setting cannot safely identify one deployment.
func configuredDiscoveryOrigin() string {
	config.OptionMapRWMutex.RLock()
	raw := strings.TrimSpace(config.ServerAddress)
	config.OptionMapRWMutex.RUnlock()
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") ||
		u.User != nil || u.Opaque != "" || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return ""
	}
	host := u.Hostname()
	if host == "" || len(host) > 253 {
		return ""
	}
	if net.ParseIP(host) == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return ""
			}
			for _, ch := range label {
				if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
					return ""
				}
			}
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return ""
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return ""
		}
	}
	return u.Scheme + "://" + u.Host
}

// discoveryOrigin returns the origin captured for c, falling back to the trusted
// administrator setting for handlers invoked without discovery middleware.
func discoveryOrigin(c *gin.Context) string {
	if origin, exists := c.Get(discoveryOriginContextKey); exists {
		return origin.(string)
	}
	return configuredDiscoveryOrigin()
}

// bindDiscoveryText replaces the hosted template origin and hostname in trusted
// discovery text with origin, returning deployment-specific text.
func bindDiscoveryText(text, origin string) string {
	host := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	return strings.NewReplacer(discoveryTemplateOrigin, origin, "oneapi.laisky.com", host).Replace(text)
}

// serveDiscoveryData writes immutable template data for c with deployment-specific
// URLs, or returns 503 when origin-bearing metadata has no valid trusted origin.
func serveDiscoveryData(c *gin.Context, contentType string, data []byte) {
	if bytes.Contains(data, []byte("oneapi.laisky.com")) {
		origin := discoveryOrigin(c)
		if origin == "" {
			c.Header("Cache-Control", "no-store")
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		data = []byte(bindDiscoveryText(string(data), origin))
	}
	// Administrative origin changes must not retain stale credential destinations.
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, contentType, data)
}

// serveDiscoveryAssets intercepts textual discovery files in buildFS before the
// static server, returning middleware that also covers flattened and nested aliases.
func serveDiscoveryAssets(buildFS fs.FS, prefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		name := strings.TrimPrefix(c.Request.URL.Path, "/")
		if !fs.ValidPath(name) {
			c.Next()
			return
		}
		contentType := ""
		switch path.Ext(name) {
		case ".json":
			contentType = "application/json; charset=utf-8"
		case ".md":
			contentType = "text/markdown; charset=utf-8"
		case ".txt":
			contentType = "text/plain; charset=utf-8"
		case ".xml":
			contentType = "application/xml; charset=utf-8"
		case ".html":
			contentType = "text/html; charset=utf-8"
		default:
			if path.Ext(name) == "" && (strings.HasPrefix(name, "well-known/") || strings.HasPrefix(name, ".well-known/")) {
				contentType = "application/json; charset=utf-8"
			} else {
				// Existing ordinary assets and directory redirects remain cacheable,
				// so they must not carry deployment-dependent discovery links.
				// Missing paths keep the Link for the no-store SPA fallback.
				if _, err := fs.Stat(buildFS, path.Join(prefix, name)); err != nil {
					c.Next()
					return
				}
				c.Header("Link", "")
				c.Next()
				return
			}
		}
		data, err := fs.ReadFile(buildFS, path.Join(prefix, name))
		if err != nil {
			// The normal static handler retains ownership of missing/unreadable files.
			c.Next()
			return
		}
		serveDiscoveryData(c, contentType, data)
		c.Abort()
	}
}
