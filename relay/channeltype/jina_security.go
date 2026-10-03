package channeltype

import (
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// JinaAllowLoopbackHTTPEnv names the explicit local-development exception.
// Public and private non-loopback HTTP endpoints are never permitted.
const JinaAllowLoopbackHTTPEnv = "ONEAPI_JINA_ALLOW_LOOPBACK_HTTP"

// ValidateJinaURL checks an absolute credential-bearing dispatch URL. It returns
// a sanitized error without echoing URL credentials or query parameters. HTTP
// requires both the operator opt-in and a literal loopback IP; DNS names are not
// trusted as loopback because their resolution can change.
func ValidateJinaURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Hostname() == "" || u.Opaque != "" || u.User != nil || u.Fragment != "" {
		return errors.New("Jina endpoint must be an absolute URL without userinfo or fragments")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return errors.New("Jina endpoint has an invalid port")
		}
	}
	if u.Scheme == "https" {
		return nil
	}
	allowed, _ := strconv.ParseBool(os.Getenv(JinaAllowLoopbackHTTPEnv))
	ip, ipErr := netip.ParseAddr(u.Hostname())
	if u.Scheme == "http" && allowed && ipErr == nil && ip.Zone() == "" && ip.Unmap().IsLoopback() {
		return nil
	}
	return errors.New("Jina endpoints require HTTPS; loopback HTTP requires explicit operator opt-in")
}

// ValidateJinaURLs validates the optional base and every configured endpoint
// override with the same dispatch policy. Empty values select built-in defaults.
// A base URL cannot have a query because native paths are appended to it.
func ValidateJinaURLs(base string, endpoints map[string]string) error {
	if strings.TrimSpace(base) != "" {
		if err := ValidateJinaURL(base); err != nil {
			return errors.Wrap(err, "validate Jina base URL")
		}
		u, _ := url.Parse(base) // Already validated above.
		if u.RawQuery != "" || u.ForceQuery {
			return errors.New("Jina base URL cannot contain a query")
		}
	}
	for _, endpoint := range endpoints {
		if strings.TrimSpace(endpoint) == "" {
			continue
		}
		if err := ValidateJinaURL(endpoint); err != nil {
			return errors.Wrap(err, "validate Jina endpoint override")
		}
	}
	return nil
}
