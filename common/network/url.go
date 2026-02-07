package network

import (
	"context"
	"net"
	"net/url"
	"strings"

	"github.com/Laisky/errors/v2"
)

// ValidateExternalURL parses rawURL, verifies the scheme and host, resolves DNS using ctx,
// and returns the parsed URL if it only maps to public IPs.
func ValidateExternalURL(ctx context.Context, rawURL string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, errors.New("url is empty")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, errors.Wrap(err, "parse url")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, errors.Errorf("unsupported url scheme: %s", parsed.Scheme)
	}

	if parsed.User != nil {
		return nil, errors.New("url must not include user info")
	}

	host := parsed.Hostname()
	if host == "" {
		return nil, errors.New("url host is empty")
	}
	if isLocalHostname(host) {
		return nil, errors.Errorf("url host is not allowed: %s", host)
	}

	if ip := net.ParseIP(host); ip != nil {
		if IsForbiddenIP(ip) {
			return nil, errors.Errorf("url host resolves to a private or local address: %s", host)
		}
		return parsed, nil
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, errors.Wrapf(err, "resolve host: %s", host)
	}
	if len(ips) == 0 {
		return nil, errors.Errorf("no IPs found for host: %s", host)
	}

	for _, addr := range ips {
		if IsForbiddenIP(addr.IP) {
			return nil, errors.Errorf("url host resolves to a private or local address: %s", host)
		}
	}

	return parsed, nil
}

// IsForbiddenIP reports whether ip is loopback, private, link-local, multicast, or otherwise non-public.
func IsForbiddenIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}

	if isCarrierGradeNAT(ip) {
		return true
	}

	return false
}

// isLocalHostname reports whether the host is a localhost-style name.
func isLocalHostname(host string) bool {
	lower := strings.ToLower(strings.TrimSuffix(host, "."))
	if lower == "localhost" {
		return true
	}
	return strings.HasSuffix(lower, ".localhost")
}

// isCarrierGradeNAT reports whether ip falls within 100.64.0.0/10.
func isCarrierGradeNAT(ip net.IP) bool {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return false
	}

	return ipv4[0] == 100 && (ipv4[1]&0xC0) == 0x40
}
