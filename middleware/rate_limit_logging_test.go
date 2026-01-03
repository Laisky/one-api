package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRateLimitMiddlewareExists verifies that rate limit middleware functions exist and can be called
func TestRateLimitMiddlewareExists(t *testing.T) {
	t.Parallel()
	t.Run("GlobalWebRateLimit_middleware_exists", func(t *testing.T) {
		t.Parallel()
		// Test that the GlobalWebRateLimit middleware function exists and can be called
		middleware := GlobalWebRateLimit()
		assert.NotNil(t, middleware, "GlobalWebRateLimit middleware should not be nil")
	})

	t.Run("GlobalAPIRateLimit_middleware_exists", func(t *testing.T) {
		t.Parallel()
		// Test that the GlobalAPIRateLimit middleware function exists and can be called
		middleware := GlobalAPIRateLimit()
		assert.NotNil(t, middleware, "GlobalAPIRateLimit middleware should not be nil")
	})

	t.Run("TotpRateLimit_middleware_exists", func(t *testing.T) {
		t.Parallel()
		// Test that the TotpRateLimit middleware function exists and can be called
		middleware := TotpRateLimit()
		assert.NotNil(t, middleware, "TotpRateLimit middleware should not be nil")
	})
}

// TestRateLimitMiddlewareFunctions verifies that rate limit middleware functions don't panic
func TestRateLimitMiddlewareFunctions(t *testing.T) {
	t.Parallel()
	t.Run("Rate_limit_functions_dont_panic", func(t *testing.T) {
		t.Parallel()
		// These functions should not panic when called
		assert.NotPanics(t, func() {
			GlobalWebRateLimit()
		}, "GlobalWebRateLimit should not panic")

		assert.NotPanics(t, func() {
			GlobalAPIRateLimit()
		}, "GlobalAPIRateLimit should not panic")

		assert.NotPanics(t, func() {
			CriticalRateLimit()
		}, "CriticalRateLimit should not panic")

		assert.NotPanics(t, func() {
			DownloadRateLimit()
		}, "DownloadRateLimit should not panic")

		assert.NotPanics(t, func() {
			UploadRateLimit()
		}, "UploadRateLimit should not panic")

		assert.NotPanics(t, func() {
			GlobalRelayRateLimit()
		}, "GlobalRelayRateLimit should not panic")

		assert.NotPanics(t, func() {
			ChannelRateLimit()
		}, "ChannelRateLimit should not panic")

		assert.NotPanics(t, func() {
			TotpRateLimit()
		}, "TotpRateLimit should not panic")
	})
}
