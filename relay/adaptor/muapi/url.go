package muapi

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// muAPIIdentifierPattern permits exactly one literal, nonempty path segment.
// The validators additionally reject dot traversal and impose bounded lengths.
var muAPIIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// muAPIBasePathPattern permits literal administrator-configured proxy prefixes.
// Encoding, delimiters, and repeated/relative segments are rejected separately.
var muAPIBasePathPattern = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)*$`)

// muAPIEndpointURL builds an upstream endpoint from an administrator-owned base
// and bounded path segments. Only the configured base supplies scheme/authority;
// identifiers cannot inject paths, queries, credentials, fragments or hosts.
// Returns a validated URL or a sanitized error before any request is dispatched.
func muAPIEndpointURL(base string, segments ...string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "https://api.muapi.ai"
	}
	// Parse drops an empty fragment. Reject its delimiter before parsing too.
	if len(base) > 8192 || strings.ContainsAny(base, "?#\\") {
		return "", errors.New("MuAPI base URL must not contain a query, fragment, or backslash")
	}
	endpoint, err := url.Parse(base)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Opaque != "" ||
		(endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		// Do not echo a malformed base: it could contain a credential.
		return "", errors.New("MuAPI base URL requires an HTTP(S) host without user information")
	}
	if port := endpoint.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("MuAPI base URL port is outside the supported range")
		}
	} else if strings.HasSuffix(endpoint.Host, ":") {
		return "", errors.New("MuAPI base URL has an empty port")
	}
	root := strings.TrimRight(endpoint.Path, "/")
	if endpoint.EscapedPath() != endpoint.Path || !muAPIBasePathPattern.MatchString(root) {
		return "", errors.New("MuAPI base URL path must be a literal proxy prefix")
	}
	for _, part := range strings.Split(root, "/") {
		if part == "." || part == ".." {
			return "", errors.New("MuAPI base URL path must not contain dot segments")
		}
	}
	// Preserve supported root, /v1 and /api/v1 forms and trusted proxy prefixes.
	for _, suffix := range []string{"/api/v1", "/v1"} {
		if strings.HasSuffix(strings.ToLower(root), suffix) {
			root = strings.TrimSuffix(root[:len(root)-len(suffix)], "/")
			break
		}
	}
	endpoint.Path = root + "/api/v1"
	for _, segment := range segments {
		if len(segment) > maxMuAPITaskIDLength || segment == "." || segment == ".." || !muAPIIdentifierPattern.MatchString(segment) {
			return "", errors.New("MuAPI endpoint requires literal non-relative path segments")
		}
		endpoint.Path += "/" + segment
	}
	return endpoint.String(), nil
}
