package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

const (
	// PaymentProviderStripe identifies Stripe Checkout as the payment provider.
	PaymentProviderStripe = "stripe"

	// PaymentStatusPending is the initial status before settlement.
	PaymentStatusPending = "pending"
	// PaymentStatusPaid means the order was settled and quota was (or is being) credited.
	PaymentStatusPaid = "paid"
	// PaymentStatusFailed marks a terminal failure (reserved for future use).
	PaymentStatusFailed = "failed"
	// PaymentStatusCanceled marks a canceled checkout (reserved for future use).
	PaymentStatusCanceled = "canceled"
)

// PaymentOrder records a top-up payment attempt against an external provider.
// SessionID is the provider-side identifier (Stripe Checkout Session id) and is
// used to make webhook handling idempotent.
type PaymentOrder struct {
	Id          int    `json:"id" gorm:"primaryKey"`
	UserId      int    `json:"user_id" gorm:"index"`
	Provider    string `json:"provider" gorm:"type:varchar(32);index"`
	SessionID   string `json:"session_id" gorm:"type:varchar(191);uniqueIndex"`
	AmountCents int64  `json:"amount_cents" gorm:"bigint"`
	Currency    string `json:"currency" gorm:"type:varchar(8)"`
	Quota       int64  `json:"quota" gorm:"bigint"`
	Status      string `json:"status" gorm:"type:varchar(16);index"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
	PaidAt      int64  `json:"paid_at" gorm:"bigint"`
}

// TableName returns the database table name for PaymentOrder.
func (PaymentOrder) TableName() string {
	return "payment_orders"
}

// CreatePaymentOrder persists a new payment order row.
// ctx scopes the database call; order must include UserId, SessionID, and Status.
// Returns a wrapped error when the insert fails.
func CreatePaymentOrder(ctx context.Context, order *PaymentOrder) error {
	if err := DB.WithContext(ctx).Create(order).Error; err != nil {
		return errors.Wrap(err, "create payment order")
	}
	return nil
}

// GetPaymentOrderBySession loads a payment order by provider session id.
// ctx scopes the database call; sessionID is the Stripe Checkout Session id.
// Returns (nil, nil) when no row exists, or a wrapped error on query failure.
func GetPaymentOrderBySession(ctx context.Context, sessionID string) (*PaymentOrder, error) {
	var order PaymentOrder
	err := DB.WithContext(ctx).Where("session_id = ?", sessionID).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "lookup payment order")
	}
	return &order, nil
}

// MarkPaymentOrderPaid transitions an order to paid state inside a transaction.
// Returns true if this call performed the transition (i.e. the order was not
// already paid). When false, the caller must NOT grant quota again.
// Deprecated for settlement: prefer SettlePaidPaymentOrder which also credits quota.
func MarkPaymentOrderPaid(ctx context.Context, sessionID string, paidAtMs int64) (transitioned bool, order *PaymentOrder, err error) {
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var found PaymentOrder
		if e := tx.Where("session_id = ?", sessionID).First(&found).Error; e != nil {
			return errors.Wrap(e, "load payment order in tx")
		}
		if found.Status == PaymentStatusPaid {
			order = &found
			return nil
		}
		if e := tx.Model(&found).Updates(map[string]any{
			"status":  PaymentStatusPaid,
			"paid_at": paidAtMs,
		}).Error; e != nil {
			return errors.Wrap(e, "mark payment order paid")
		}
		found.Status = PaymentStatusPaid
		found.PaidAt = paidAtMs
		transitioned = true
		order = &found
		return nil
	})
	if err != nil {
		return false, nil, err
	}
	return transitioned, order, nil
}

// SettlePaidPaymentOrder marks a pending order paid and credits the user quota in one transaction.
// ctx scopes the database work; sessionID is the Stripe Checkout Session id; paidAtMs is the paid timestamp in milliseconds.
// Returns transitioned=false when the order was already paid (idempotent no-op).
// Returns a wrapped error when the order is missing or the transaction fails.
func SettlePaidPaymentOrder(ctx context.Context, sessionID string, paidAtMs int64) (transitioned bool, order *PaymentOrder, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var found PaymentOrder
		if e := tx.Where("session_id = ?", sessionID).First(&found).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return errors.Wrap(e, "payment order not found")
			}
			return errors.Wrap(e, "load payment order in settle tx")
		}
		if found.Status == PaymentStatusPaid {
			order = &found
			return nil
		}
		if e := tx.Model(&found).Updates(map[string]any{
			"status":  PaymentStatusPaid,
			"paid_at": paidAtMs,
		}).Error; e != nil {
			return errors.Wrap(e, "mark payment order paid in settle tx")
		}
		// Credit quota in the same transaction so a crash cannot leave paid-without-quota.
		// Bypass BatchUpdateEnabled so settlement is always durable in-tx.
		if found.Quota < 0 {
			return errors.New("quota cannot be negative")
		}
		if found.Quota > 0 {
			if e := tx.Model(&User{}).Where("id = ?", found.UserId).
				Update("quota", gorm.Expr("quota + ?", found.Quota)).Error; e != nil {
				return errors.Wrapf(e, "increase quota for user %d in settle tx", found.UserId)
			}
		}
		found.Status = PaymentStatusPaid
		found.PaidAt = paidAtMs
		transitioned = true
		order = &found
		return nil
	})
	if err != nil {
		return false, nil, err
	}
	return transitioned, order, nil
}
