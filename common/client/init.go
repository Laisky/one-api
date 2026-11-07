package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	netutil "github.com/Laisky/one-api/common/network"
)

// HTTPClient is the default outbound client used for relay requests.
var HTTPClient *http.Client

// ImpatientHTTPClient is a short-timeout client for quick health checks or metadata requests.
var ImpatientHTTPClient *http.Client

// UserContentRequestHTTPClient fetches user-supplied resources with strict limits to reduce SSRF/DoS risk.
var UserContentRequestHTTPClient *http.Client

// buildUserContentDialContext enforces that outbound connections only target public IPs.
// Parameters: proxyURL is the optional proxy address; returns a DialContext function for http.Transport.
func buildUserContentDialContext(proxyURL *url.URL) func(ctx context.Context, networkName string, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	proxyHost := ""
	if proxyURL != nil {
		proxyHost = strings.ToLower(proxyURL.Hostname())
	}

	return func(ctx context.Context, networkName string, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, errors.Wrapf(err, "split host and port: %s", addr)
		}

		if proxyHost != "" && strings.EqualFold(host, proxyHost) {
			return dialer.DialContext(ctx, networkName, addr)
		}

		if ip := net.ParseIP(host); ip != nil {
			if netutil.IsForbiddenIP(ip) {
				return nil, errors.Errorf("blocked private address: %s", host)
			}
			return dialer.DialContext(ctx, networkName, net.JoinHostPort(ip.String(), port))
		}

		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, errors.Wrapf(err, "resolve host: %s", host)
		}
		if len(ips) == 0 {
			return nil, errors.Errorf("no IPs found for host: %s", host)
		}

		for _, addr := range ips {
			if netutil.IsForbiddenIP(addr.IP) {
				return nil, errors.Errorf("blocked private address for host: %s", host)
			}
		}

		return dialer.DialContext(ctx, networkName, net.JoinHostPort(ips[0].IP.String(), port))
	}
}

// userContentProxy returns the Transport.Proxy hook of the user-content client
// behind USER_CONTENT_REQUEST_PROXY. The transport calls it before connecting
// for every request, each redirect hop included. Behind a proxy the dial guard
// only sees the proxy's own address, so this hook applies the public-address
// policy to the request target instead; that covers every caller of the shared
// client, including ones that do not validate the URL themselves. The proxy
// still resolves the name again, so it must enforce its own egress policy
// against DNS rebinding. Parameters: proxyURL is the configured proxy. Returns
// a hook yielding proxyURL for an allowed target and an error otherwise.
func userContentProxy(proxyURL *url.URL) func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		if _, err := netutil.ValidateExternalURL(req.Context(), req.URL.String()); err != nil {
			return nil, errors.Wrap(err, "user content target not allowed")
		}
		return proxyURL, nil
	}
}

// Init builds the shared HTTP clients with proxy and timeout settings derived from configuration.
func Init() {
	// Create a transport with HTTP/2 disabled to avoid stream errors in CI environments
	createTransport := func(proxyURL *url.URL, restrictExternal bool) *http.Transport {
		transport := &http.Transport{
			TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper), // Disable HTTP/2
		}
		if proxyURL != nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
		if restrictExternal {
			transport.DialContext = buildUserContentDialContext(proxyURL)
			if proxyURL != nil {
				transport.Proxy = userContentProxy(proxyURL)
			}
		}
		return transport
	}

	if config.UserContentRequestProxy != "" {
		logger.Logger.Info("using proxy to fetch user content", zap.String("proxy", logger.Redact(config.UserContentRequestProxy)))
		proxyURL, err := url.Parse(config.UserContentRequestProxy)
		if err != nil {
			// The configured URL and parser error can both include proxy credentials.
			logger.Logger.Fatal("USER_CONTENT_REQUEST_PROXY set but invalid",
				zap.String("proxy", logger.Redact(config.UserContentRequestProxy)),
				zap.String("error", logger.Redact(err)),
				zap.String("error_type", fmt.Sprintf("%T", err)))
		}
		UserContentRequestHTTPClient = &http.Client{
			Transport: createTransport(proxyURL, true),
			Timeout:   time.Second * time.Duration(config.UserContentRequestTimeout),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("stopped after 5 redirects")
				}
				if _, err := netutil.ValidateExternalURL(req.Context(), req.URL.String()); err != nil {
					return errors.Wrap(err, "redirect target not allowed")
				}
				return nil
			},
		}
	} else {
		UserContentRequestHTTPClient = &http.Client{
			Transport: createTransport(nil, true),
			Timeout:   30 * time.Second, // Set a reasonable default timeout
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("stopped after 5 redirects")
				}
				if _, err := netutil.ValidateExternalURL(req.Context(), req.URL.String()); err != nil {
					return errors.Wrap(err, "redirect target not allowed")
				}
				return nil
			},
		}
	}
	var transport http.RoundTripper
	if config.RelayProxy != "" {
		logger.Logger.Info("using api relay proxy", zap.String("proxy", logger.Redact(config.RelayProxy)))
		proxyURL, err := url.Parse(config.RelayProxy)
		if err != nil {
			// The configured URL and parser error can both include proxy credentials.
			logger.Logger.Fatal("RELAY_PROXY set but invalid",
				zap.String("proxy", logger.Redact(config.RelayProxy)),
				zap.String("error", logger.Redact(err)),
				zap.String("error_type", fmt.Sprintf("%T", err)))
		}
		transport = createTransport(proxyURL, false)
	} else {
		transport = createTransport(nil, false)
	}

	if config.RelayTimeout == 0 {
		HTTPClient = &http.Client{
			Transport: transport,
		}
	} else {
		HTTPClient = &http.Client{
			Timeout:   time.Duration(config.RelayTimeout) * time.Second,
			Transport: transport,
		}
	}

	ImpatientHTTPClient = &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
}
