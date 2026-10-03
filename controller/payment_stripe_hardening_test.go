package controller

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

func TestStripeAmountCentsPreservesMinorUnits(t *testing.T) {
	tests := []struct {
		name    string
		amount  float64
		minUSD  int
		want    int64
		wantErr string
	}{
		{name: "whole dollars", amount: 10, minUSD: 1, want: 1000},
		{name: "two decimal places", amount: 19.99, minUSD: 1, want: 1999},
		{name: "minimum boundary", amount: 5, minUSD: 5, want: 500},
		{name: "three decimal places rejected", amount: 1.001, minUSD: 1, wantErr: "two decimal"},
		{name: "below minimum", amount: 4.99, minUSD: 5, wantErr: "minimum"},
		{name: "above maximum", amount: 100000.01, minUSD: 1, wantErr: "too large"},
		{name: "not a number", amount: math.NaN(), minUSD: 1, wantErr: "finite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stripeAmountCents(tt.amount, tt.minUSD)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestStripePublicBaseURLRequiresTrustedOrigin(t *testing.T) {
	previousKey := config.StripeSecretKey
	previousBase := config.StripePublicBaseURL
	previousServer := config.ServerAddress
	t.Cleanup(func() {
		config.StripeSecretKey = previousKey
		config.StripePublicBaseURL = previousBase
		config.ServerAddress = previousServer
	})

	config.ServerAddress = ""
	config.StripeSecretKey = "sk_test_example"

	config.StripePublicBaseURL = "https://pay.example.com/"
	base, err := stripePublicBaseURL(true)
	require.NoError(t, err)
	require.Equal(t, "https://pay.example.com", base)

	config.StripePublicBaseURL = "http://localhost:3000"
	base, err = stripePublicBaseURL(true)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:3000", base)

	for _, invalid := range []string{
		"http://pay.example.com",
		"https://user:pass@pay.example.com",
		"https://pay.example.com/app",
		"https://pay.example.com?next=evil",
		"javascript:alert(1)",
	} {
		config.StripePublicBaseURL = invalid
		_, err = stripePublicBaseURL(true)
		require.Error(t, err, invalid)
	}

	config.StripeSecretKey = "sk_live_example"
	config.StripePublicBaseURL = "http://127.0.0.1:8080"
	_, err = stripePublicBaseURL(true)
	require.Error(t, err)
}

func TestStripeRequestIdentityIsScopedAndStable(t *testing.T) {
	requestID := "550e8400-e29b-41d4-a716-446655440000"
	require.True(t, validStripeRequestID(requestID))
	require.False(t, validStripeRequestID("contains spaces"))
	require.False(t, validStripeRequestID(strings.Repeat("x", 65)))

	require.Equal(t, "one-api:stripe:checkout:v1:7:"+requestID, stripeIdempotencyKey(7, requestID))
	require.NotEqual(t, stripeIdempotencyKey(7, requestID), stripeIdempotencyKey(8, requestID))
	require.NotEqual(t, stripePendingSession(7, requestID), stripePendingSession(8, requestID))
}

func TestPaymentOrderMatchesRejectsIdempotencyDrift(t *testing.T) {
	order := &model.PaymentOrder{
		Provider:    model.PaymentProviderStripe,
		AmountCents: 1999,
		Currency:    "usd",
		Quota:       19990,
		Status:      model.PaymentStatusPending,
	}
	require.True(t, paymentOrderMatches(order, 1999, 19990))
	require.False(t, paymentOrderMatches(order, 2000, 19990))
	require.False(t, paymentOrderMatches(order, 1999, 20000))

	order.Status = model.PaymentStatusPaid
	require.False(t, paymentOrderMatches(order, 1999, 19990))
}
