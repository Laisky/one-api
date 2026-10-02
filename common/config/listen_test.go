package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAPIListenAddress preserves default port behavior and accepts explicit IPv4/IPv6 hosts.
func TestAPIListenAddress(t *testing.T) {
	for _, tc := range []struct{ host, port, want string }{
		{"", "3000", ":3000"},
		{"  ", "0", ":0"},
		{"127.0.0.1", "8080", "127.0.0.1:8080"},
		{" ::1 ", "8080", "[::1]:8080"},
		{"0.0.0.0", "3000", "0.0.0.0:3000"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("LISTEN_HOST", tc.host)
			require.Equal(t, tc.want, APIListenAddress(tc.port))
		})
	}
}
