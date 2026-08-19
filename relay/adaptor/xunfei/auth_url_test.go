package xunfei

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildXunfeiAuthURLRejectsMalformedEndpoint verifies that URL construction
// returns a wrapped error instead of continuing with an unusable signed endpoint.
func TestBuildXunfeiAuthURLRejectsMalformedEndpoint(t *testing.T) {
	t.Parallel()

	signedURL, err := buildXunfeiAuthUrl("://malformed", "test-key", "test-secret")

	require.Error(t, err)
	require.Empty(t, signedURL)
	require.Contains(t, err.Error(), "parse Xunfei websocket URL")
}

// TestGetXunfeiAuthURLBuildsKnownEndpoint verifies that the provider-specific
// endpoint and authorization query are preserved after error propagation was added.
func TestGetXunfeiAuthURLBuildsKnownEndpoint(t *testing.T) {
	t.Parallel()

	domain, signedURL, err := getXunfeiAuthUrl("v3.1-128K", "test-key", "test-secret")

	require.NoError(t, err)
	require.Equal(t, "pro-128k", domain)
	parsed, err := url.Parse(signedURL)
	require.NoError(t, err)
	require.Equal(t, "spark-api.xf-yun.com", parsed.Host)
	require.Equal(t, "/chat/pro-128k", parsed.Path)
	require.NotEmpty(t, parsed.Query().Get("authorization"))
	require.NotEmpty(t, parsed.Query().Get("date"))
	require.Equal(t, "spark-api.xf-yun.com", parsed.Query().Get("host"))
}
