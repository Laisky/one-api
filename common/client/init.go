package client

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/network"
)

// HTTPClient is the default outbound client used for relay requests.
var HTTPClient *http.Client

// ImpatientHTTPClient is a short-timeout client for quick health checks or metadata requests.
var ImpatientHTTPClient *http.Client

// UserContentRequestHTTPClient fetches user-supplied resources, optionally via a dedicated proxy.
var UserContentRequestHTTPClient *http.Client

// Init builds the shared HTTP clients with proxy and timeout settings derived from configuration.
func Init() {
	// Create a transport with HTTP/2 disabled to avoid stream errors in CI environments.
	// Optionally blocks internal IP addresses to mitigate SSRF risks.
	createTransport := func(proxyURL *url.URL, blockInternal bool) *http.Transport {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		if blockInternal {
			dialer.Control = func(networkName, address string, c syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return errors.Wrapf(err, "failed to split host port: %s", address)
				}
				ip := net.ParseIP(host)
				if ip == nil {
					return errors.Errorf("SSRF protection: failed to parse IP address: %s", host)
				}
				if network.IsInternalIP(ip) {
					return errors.Errorf("SSRF protection: internal IP %s is blocked", ip)
				}
				return nil
			}
		}

		transport := &http.Transport{
			DialContext:  dialer.DialContext,
			TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper), // Disable HTTP/2
		}
		if proxyURL != nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
		return transport
	}

	if config.UserContentRequestProxy != "" {
		logger.Logger.Info("using proxy to fetch user content", zap.String("proxy", config.UserContentRequestProxy))
		proxyURL, err := url.Parse(config.UserContentRequestProxy)
		if err != nil {
			logger.Logger.Fatal(fmt.Sprintf("USER_CONTENT_REQUEST_PROXY set but invalid: %s", config.UserContentRequestProxy))
		}
		UserContentRequestHTTPClient = &http.Client{
			Transport: createTransport(proxyURL, config.BlockInternalUserContentRequests),
			Timeout:   time.Second * time.Duration(config.UserContentRequestTimeout),
		}
	} else {
		UserContentRequestHTTPClient = &http.Client{
			Transport: createTransport(nil, config.BlockInternalUserContentRequests),
			Timeout:   time.Second * time.Duration(config.UserContentRequestTimeout),
		}
	}
	var transport http.RoundTripper
	if config.RelayProxy != "" {
		logger.Logger.Info("using api relay proxy", zap.String("proxy", config.RelayProxy))
		proxyURL, err := url.Parse(config.RelayProxy)
		if err != nil {
			logger.Logger.Fatal(fmt.Sprintf("RELAY_PROXY set but invalid: %s", config.RelayProxy))
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
