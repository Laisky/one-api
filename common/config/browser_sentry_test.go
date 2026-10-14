package config

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

const browserConfigFixture = `{"dsn":"https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456","environment":"production"}`

// TestParseBrowserSentryConfig accepts only bounded public DSN and environment fields, rejecting credentials, release overrides, duplicate keys, and malformed URLs without echoing values.
func TestParseBrowserSentryConfig(t *testing.T) {
	valid, err := ParseBrowserSentryConfig(browserConfigFixture)
	require.NoError(t, err)
	require.Equal(t, "production", valid.Environment)
	empty, err := ParseBrowserSentryConfig("  ")
	require.NoError(t, err)
	require.Nil(t, empty)
	for _, raw := range []string{
		"null", "[]", "{}", strings.Repeat("x", 1025), browserConfigFixture + "{}",
		strings.TrimSuffix(browserConfigFixture, "}") + `,"release":"secret"}`,
		strings.TrimSuffix(browserConfigFixture, "}") + `,"environment":"staging"}`,
		strings.Replace(browserConfigFixture, `"production"`, `false`, 1),
		strings.Replace(browserConfigFixture, "production", "secret", 1),
		strings.Replace(browserConfigFixture, "https:", "http:", 1),
		strings.Replace(browserConfigFixture, "@o123", ":secret@o123", 1),
		strings.Replace(browserConfigFixture, "sentry.io", "sentry.io.evil.test", 1),
		strings.Replace(browserConfigFixture, "/456", "/456?token=secret", 1),
		strings.Replace(browserConfigFixture, "/456", "/456#secret", 1),
		strings.Replace(browserConfigFixture, "/456", "/%34%35%36", 1),
	} {
		value, err := ParseBrowserSentryConfig(raw)
		require.Error(t, err)
		require.Nil(t, value)
		require.NotContains(t, err.Error(), "secret")
		require.NotContains(t, err.Error(), "https://")
	}
}
