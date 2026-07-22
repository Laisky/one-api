package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestStripeReadyRequiresSecretsAndBaseURL documents the readiness gate for Checkout.
func TestStripeReadyRequiresSecretsAndBaseURL(t *testing.T) {
	prevKey := config.StripeSecretKey
	prevWh := config.StripeWebhookSecret
	prevBase := config.StripePublicBaseURL
	prevServer := config.ServerAddress
	t.Cleanup(func() {
		config.StripeSecretKey = prevKey
		config.StripeWebhookSecret = prevWh
		config.StripePublicBaseURL = prevBase
		config.ServerAddress = prevServer
	})

	config.StripeSecretKey = ""
	config.StripeWebhookSecret = ""
	config.StripePublicBaseURL = ""
	config.ServerAddress = ""
	require.False(t, StripeReady())

	config.StripeSecretKey = "sk_test_x"
	require.False(t, StripeReady())

	config.StripeWebhookSecret = "whsec_x"
	config.ServerAddress = "https://example.com"
	require.True(t, StripeReady())

	config.StripeSecretKey = "sk_live_x"
	config.ServerAddress = "http://insecure.example.com"
	config.StripePublicBaseURL = ""
	require.False(t, StripeReady())

	config.StripePublicBaseURL = "https://pay.example.com"
	require.True(t, StripeReady())
}
