package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPaymentProviderStripeConstant ensures the Stripe provider constant used by
// checkout and webhook handlers remains stable for VPS billing migrations.
func TestPaymentProviderStripeConstant(t *testing.T) {
	require.Equal(t, "stripe", PaymentProviderStripe)
}

// TestPaymentOrderTableName documents the GORM table name for restore tooling.
func TestPaymentOrderTableName(t *testing.T) {
	var o PaymentOrder
	require.Equal(t, "payment_orders", o.TableName())
}
