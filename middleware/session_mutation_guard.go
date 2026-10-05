package middleware

import (
	"net/http"
	"net/url"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
)

// SessionMutationGuard returns middleware that requires trustworthy browser
// provenance for unsafe requests carrying an authenticated dashboard session.
// Bearer-only clients remain independent of browser provenance. A bearer header
// never exempts a request whose signed cookie is authoritative for authentication.
func SessionMutationGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if allowSessionMutation(c) {
			c.Next()
		}
	}
}

// allowSessionMutation validates cookie-authorized mutation provenance for c,
// writes a forbidden response on rejection and returns whether to proceed.
func allowSessionMutation(c *gin.Context) bool {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions || sessions.Default(c).Get("username") == nil {
		return true
	}
	if trustedSessionMutation(c.Request) {
		return true
	}
	lg := gmw.GetLogger(c)
	lg.Debug("rejected dashboard session mutation provenance", zap.String("method", c.Request.Method), zap.String("route", c.FullPath()))
	respondAuthError(c, http.StatusForbidden, "Untrusted session mutation origin")
	return false
}

// trustedSessionMutation reports whether request provenance names the effective
// API origin or an explicitly configured public API/frontend origin. Missing
// Origin falls back to same-origin Fetch Metadata or an exact trusted Referer.
// Untrusted forwarding headers never define an allowed origin.
func trustedSessionMutation(request *http.Request) bool {
	origins := request.Header.Values("Origin")
	sites := request.Header.Values("Sec-Fetch-Site")
	if len(origins) > 1 || len(sites) > 1 {
		return false
	}
	if len(origins) == 1 && origins[0] != "" {
		origin := sessionOrigin(origins[0], false)
		return origin != "" && trustedSessionOrigin(request, origin)
	}
	site := request.Header.Get("Sec-Fetch-Site")
	if site == "same-origin" {
		return true
	}
	// Same-site is weaker than same-origin; none and cross-site are not proof
	// of an intentional dashboard action. Configured external frontends must
	// send their explicit Origin, as browser fetch requests normally do.
	if site != "" {
		return false
	}
	referers := request.Header.Values("Referer")
	if len(referers) != 1 {
		return false
	}
	origin := sessionOrigin(referers[0], true)
	return origin != "" && trustedSessionOrigin(request, origin)
}

// trustedSessionOrigin compares canonical origin with the request and configured
// origins, including schemes and ports. Secure-cookie deployments use HTTPS
// behind TLS termination; HTTP development explicitly disables secure cookies.
func trustedSessionOrigin(request *http.Request, origin string) bool {
	scheme := "http"
	if request.TLS != nil || config.EnableCookieSecure {
		scheme = "https"
	}
	candidates := []string{scheme + "://" + request.Host, config.FrontendBaseURL}
	// ServerAddress has a development placeholder, which is not an explicit
	// trust grant to a localhost website on a production user's machine.
	if config.ServerAddress != "http://localhost:3000" {
		candidates = append(candidates, config.ServerAddress)
	}
	for _, candidate := range candidates {
		if normalized := sessionOrigin(candidate, true); normalized != "" && normalized == origin {
			return true
		}
	}
	return false
}

// sessionOrigin returns a canonical HTTP(S) origin for raw, or an empty string
// for invalid input. allowPath permits paths on configuration URLs and Referer;
// an Origin header must contain only a scheme and authority.
func sessionOrigin(raw string, allowPath bool) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	if !allowPath && (u.Path != "" || u.RawQuery != "" || u.ForceQuery) {
		return ""
	}
	host := strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		host = strings.TrimSuffix(host, ":"+u.Port())
	}
	return u.Scheme + "://" + host
}
