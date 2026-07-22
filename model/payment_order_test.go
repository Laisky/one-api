package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestPaymentProviderStripeConstant ensures the Stripe provider constant remains stable.
func TestPaymentProviderStripeConstant(t *testing.T) {
	require.Equal(t, "stripe", PaymentProviderStripe)
}

// TestPaymentOrderTableName documents the GORM table name for restore tooling.
func TestPaymentOrderTableName(t *testing.T) {
	var o PaymentOrder
	require.Equal(t, "payment_orders", o.TableName())
}

// setupPaymentTestDB installs an in-memory SQLite DB for payment settlement tests.
func setupPaymentTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	prev := DB
	DB = db
	t.Cleanup(func() { DB = prev })
	require.NoError(t, db.AutoMigrate(&PaymentOrder{}, &StripeWebhookEvent{}, &User{}))
}

// TestSettlePaidPaymentOrderIdempotent credits quota only once across concurrent claims.
func TestSettlePaidPaymentOrderIdempotent(t *testing.T) {
	setupPaymentTestDB(t)
	user := &User{Id: 1, Username: "pay-user", Password: "x", DisplayName: "p", Role: 1, Status: 1, Quota: 100}
	require.NoError(t, DB.Create(user).Error)
	order := &PaymentOrder{
		UserId:      1,
		Provider:    PaymentProviderStripe,
		RequestID:   "req-1",
		SessionID:   "cs_test_1",
		AmountCents: 500,
		Currency:    "usd",
		Quota:       50,
		Status:      PaymentStatusPending,
	}
	require.NoError(t, DB.Create(order).Error)

	ctx := context.Background()
	now := time.Now().UnixMilli()
	tr1, o1, err1 := SettlePaidPaymentOrder(ctx, "cs_test_1", now, 500, "usd")
	require.NoError(t, err1)
	require.True(t, tr1)
	require.NotNil(t, o1)

	tr2, o2, err2 := SettlePaidPaymentOrder(ctx, "cs_test_1", now+1, 500, "usd")
	require.NoError(t, err2)
	require.False(t, tr2)
	require.NotNil(t, o2)
	require.Equal(t, PaymentStatusPaid, o2.Status)

	var u User
	require.NoError(t, DB.First(&u, 1).Error)
	require.Equal(t, int64(150), u.Quota)
}

// TestSettlePaidPaymentOrderUnknownSession returns ErrPaymentOrderNotFound.
func TestSettlePaidPaymentOrderUnknownSession(t *testing.T) {
	setupPaymentTestDB(t)
	_, order, err := SettlePaidPaymentOrder(context.Background(), "cs_missing", time.Now().UnixMilli(), 0, "")
	require.ErrorIs(t, err, ErrPaymentOrderNotFound)
	require.Nil(t, order)
}

// TestSettlePaidPaymentOrderAmountMismatch rejects Stripe amount drift.
func TestSettlePaidPaymentOrderAmountMismatch(t *testing.T) {
	setupPaymentTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 2, Username: "u2", Password: "x", DisplayName: "p", Role: 1, Status: 1, Quota: 0}).Error)
	require.NoError(t, DB.Create(&PaymentOrder{
		UserId: 2, Provider: PaymentProviderStripe, RequestID: "req-2", SessionID: "cs_2",
		AmountCents: 500, Currency: "usd", Quota: 10, Status: PaymentStatusPending,
	}).Error)
	_, _, err := SettlePaidPaymentOrder(context.Background(), "cs_2", time.Now().UnixMilli(), 999, "usd")
	require.Error(t, err)
	require.Contains(t, err.Error(), "amount mismatch")
}

// TestClaimStripeWebhookEventDedupes inserts an event id only once.
func TestClaimStripeWebhookEventDedupes(t *testing.T) {
	setupPaymentTestDB(t)
	ctx := context.Background()
	ok1, err1 := ClaimStripeWebhookEvent(ctx, "evt_1", "checkout.session.completed")
	require.NoError(t, err1)
	require.True(t, ok1)
	ok2, err2 := ClaimStripeWebhookEvent(ctx, "evt_1", "checkout.session.completed")
	require.NoError(t, err2)
	require.False(t, ok2)
}

// TestSettleMissingUserManualReview marks manual_review when the user row is gone.
func TestSettleMissingUserManualReview(t *testing.T) {
	setupPaymentTestDB(t)
	require.NoError(t, DB.Create(&PaymentOrder{
		UserId: 99, Provider: PaymentProviderStripe, RequestID: "req-miss", SessionID: "cs_miss",
		AmountCents: 100, Currency: "usd", Quota: 10, Status: PaymentStatusPending,
	}).Error)
	tr, order, err := SettlePaidPaymentOrder(context.Background(), "cs_miss", time.Now().UnixMilli(), 100, "usd")
	require.NoError(t, err)
	require.False(t, tr)
	require.NotNil(t, order)
	require.Equal(t, PaymentStatusManualReview, order.Status)
}
