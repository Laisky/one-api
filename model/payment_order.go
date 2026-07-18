package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

const (
	PaymentProviderStripe = "stripe"

	PaymentStatusPending  = "pending"
	PaymentStatusPaid     = "paid"
	PaymentStatusFailed   = "failed"
	PaymentStatusCanceled = "canceled"
)

// PaymentOrder records a top-up payment attempt against an external provider.
//
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

func (PaymentOrder) TableName() string {
	return "payment_orders"
}

func CreatePaymentOrder(ctx context.Context, order *PaymentOrder) error {
	if err := DB.WithContext(ctx).Create(order).Error; err != nil {
		return errors.Wrap(err, "create payment order")
	}
	return nil
}

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
