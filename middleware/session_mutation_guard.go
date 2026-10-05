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

// PendingLoginMutationGuard returns middleware that requires trustworthy
// browser provenance for every unsafe request, even before a dashboard session
// exists. It protects endpoints whose credential is a pre-authentication marker
// in the signed session cookie, such as completing a pending OAuth login with
// a TOTP code, using the same Fetch Metadata, Origin and Referer rules as
// SessionMutationGuard.
func PendingLoginMutationGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSafeMethod(c.Request.Method) || trustedSessionMutation(c.Request) {
			c.Next()
			return
		}
		lg := gmw.GetLogger(c)
		lg.Debug("rejected pending login mutation provenance", zap.String("method", c.Request.Method), zap.String("route", c.FullPath()))
		respondAuthError(c, http.StatusForbidden, "Untrusted session mutation origin")
	}
}

// SessionWriteNavigationGuard returns middleware for safe-method endpoints that
// still write the signed session, such as OAuth state issuance. A hostile
// top-level navigation carries the Lax session cookie and its response may set
// a new one, so requests that Fetch Metadata marks as cross-site or same-site
// are rejected unless their Origin is a trusted dashboard origin (a configured
// external frontend). Same-origin requests, user-initiated navigations
// ("none") and clients that send no Fetch Metadata keep working.
func SessionWriteNavigationGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if trustedSessionWrite(c.Request) {
			c.Next()
			return
		}
		lg := gmw.GetLogger(c)
		lg.Debug("rejected cross-site session write", zap.String("method", c.Request.Method), zap.String("route", c.FullPath()))
		respondAuthError(c, http.StatusForbidden, "Untrusted session request origin")
	}
}

// trustedSessionWrite reports whether request may write the session from a
// safe-method endpoint. It returns false for duplicated Fetch Metadata and for
// cross-site or same-site requests without a trusted Origin header.
func trustedSessionWrite(request *http.Request) bool {
	sites := request.Header.Values("Sec-Fetch-Site")
	if len(sites) > 1 {
		return false
	}
	switch request.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	origins := request.Header.Values("Origin")
	if len(origins) != 1 || origins[0] == "" {
		return false
	}
	origin := sessionOrigin(origins[0], false)
	return origin != "" && trustedSessionOrigin(request, origin)
}

// isSafeMethod reports whether method is a read-only HTTP method that never
// carries a dashboard mutation.
func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// allowSessionMutation validates cookie-authorized mutation provenance for c,
// writes a forbidden response on rejection and returns whether to proceed.
func allowSessionMutation(c *gin.Context) bool {
	if isSafeMethod(c.Request.Method) || sessions.Default(c).Get("username") == nil {
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

// trustedSessionMutation reports whether request provenance proves a
// same-origin browser request or names an explicitly trusted origin. Fetch
// Metadata is checked first, as the OWASP CSRF guidance recommends: browsers
// set Sec-Fetch-Site themselves and pages cannot forge it, and same-origin
// stays correct when a proxy rewrites Host or terminates TLS. Otherwise the
// Origin must be the effective API origin or a configured public API/frontend
// origin. Only when both headers are absent does an exact trusted Referer
// count. Untrusted forwarding headers never define an allowed origin.
func trustedSessionMutation(request *http.Request) bool {
	origins := request.Header.Values("Origin")
	sites := request.Header.Values("Sec-Fetch-Site")
	if len(origins) > 1 || len(sites) > 1 {
		return false
	}
	site := request.Header.Get("Sec-Fetch-Site")
	if site == "same-origin" {
		return true
	}
	if len(origins) == 1 && origins[0] != "" {
		origin := sessionOrigin(origins[0], false)
		return origin != "" && trustedSessionOrigin(request, origin)
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
	config.OptionMapRWMutex.RLock()
	serverAddress := config.ServerAddress
	config.OptionMapRWMutex.RUnlock()
	// ServerAddress has a development placeholder, which is not an explicit
	// trust grant to a localhost website on a production user's machine.
	if serverAddress != "http://localhost:3000" {
		candidates = append(candidates, serverAddress)
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
