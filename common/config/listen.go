package config

import (
	"net"
	"strings"

	"github.com/Laisky/one-api/common/env"
)

// APIListenAddress returns the API listen address for port. LISTEN_HOST is an optional
// host override; its empty default preserves the existing all-interface listener.
// Explicit loopback binding is useful for local diagnostics and private fixtures.
func APIListenAddress(port string) string {
	return net.JoinHostPort(strings.TrimSpace(env.String("LISTEN_HOST", "")), port)
}
