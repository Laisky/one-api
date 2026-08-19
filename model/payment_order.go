package model

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

const (
	// PaymentProviderStripe identifies Stripe Checkout as the payment provider.
	PaymentProviderStripe = "stripe"

	// PaymentStatusPending is the initial status before settlement.
	PaymentStatusPending = "pending"
	// PaymentStatusPaid means the order was settled and quota was credited.
	PaymentStatusPaid = "paid"
	// PaymentStatusFailed marks a terminal failure.
	PaymentStatusFailed = "failed"
	// PaymentStatusCanceled marks a canceled or expired checkout.
	PaymentStatusCanceled = "canceled"
	// PaymentStatusManualReview needs operator attention (e.g. missing user).
	PaymentStatusManualReview = "manual_review"
)

// ErrPaymentOrderNotFound is returned when a Stripe session has no local order.
var ErrPaymentOrderNotFound = errors.New("payment order not found")

// PaymentOrder records a top-up payment attempt against an external provider.
// SessionID is the provider-side identifier (Stripe Checkout Session id) and is
// used to make webhook handling idempotent. RequestID is a local idempotency key
// for checkout creation retries.
type PaymentOrder struct {
	Id          int    `json:"id" gorm:"primaryKey"`
	UserId      int    `json:"user_id" gorm:"uniqueIndex:idx_payment_orders_user_request,priority:1"`
	Provider    string `json:"provider" gorm:"type:varchar(32);index"`
	RequestID   string `json:"request_id" gorm:"type:varchar(64);uniqueIndex:idx_payment_orders_user_request,priority:2"`
	SessionID   string `json:"session_id" gorm:"type:varchar(191);uniqueIndex"`
	AmountCents int64  `json:"amount_cents" gorm:"bigint"`
	Currency    string `json:"currency" gorm:"type:varchar(8)"`
	Quota       int64  `json:"quota" gorm:"bigint"`
	Status      string `json:"status" gorm:"type:varchar(32);index"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
	PaidAt      int64  `json:"paid_at" gorm:"bigint"`
}

// TableName returns the database table name for PaymentOrder.
func (PaymentOrder) TableName() string {
	return "payment_orders"
}

// StripeWebhookEvent records a processed Stripe event id for webhook deduplication.
type StripeWebhookEvent struct {
	EventID   string `json:"event_id" gorm:"primaryKey;type:varchar(191)"`
	EventType string `json:"event_type" gorm:"type:varchar(64)"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
}

// TableName returns the database table name for StripeWebhookEvent.
func (StripeWebhookEvent) TableName() string {
	return "stripe_webhook_events"
}

// NewPaymentRequestID generates a random local idempotency key for checkout creation.
func NewPaymentRequestID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errors.Wrap(err, "generate payment request id")
	}
	return hex.EncodeToString(b[:]), nil
}

// CreatePaymentOrder persists a new payment order row.
// ctx scopes the database call; order must include UserId, SessionID, RequestID, and Status.
// Returns a wrapped error when the insert fails.
func CreatePaymentOrder(ctx context.Context, order *PaymentOrder) error {
	if err := DB.WithContext(ctx).Create(order).Error; err != nil {
		return errors.Wrap(err, "create payment order")
	}
	return nil
}

// BindPaymentOrderSession sets the Stripe session id on a pending local order.
// ctx scopes the update; orderID is the local primary key; sessionID is the Stripe Checkout Session id.
// Returns a wrapped error when the update fails. Idempotent when already bound to the same session.
func BindPaymentOrderSession(ctx context.Context, orderID int, sessionID string) error {
	var found PaymentOrder
	if err := DB.WithContext(ctx).Where("id = ?", orderID).First(&found).Error; err != nil {
		return errors.Wrap(err, "load payment order for session bind")
	}
	if found.SessionID == sessionID {
		return nil
	}
	if found.Status != PaymentStatusPending {
		return errors.Errorf("payment order %d is not pending", orderID)
	}
	res := DB.WithContext(ctx).Model(&found).
		Where("id = ? AND status = ?", orderID, PaymentStatusPending).
		Update("session_id", sessionID)
	if res.Error != nil {
		return errors.Wrap(res.Error, "bind payment order session")
	}
	if res.RowsAffected == 0 {
		return errors.New("payment order not pending for session bind")
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

// GetPaymentOrderByRequestID loads a payment order by local request id and owning user id.
// Returns (nil, nil) when no matching row exists.
func GetPaymentOrderByRequestID(ctx context.Context, userID int, requestID string) (*PaymentOrder, error) {
	var order PaymentOrder
	err := DB.WithContext(ctx).Where("user_id = ? AND request_id = ?", userID, requestID).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "lookup payment order by request id")
	}
	return &order, nil
}

// GetPaymentOrderBySessionForUser loads a payment order by session id and owning user id.
// Returns (nil, nil) when no matching row exists.
func GetPaymentOrderBySessionForUser(ctx context.Context, userID int, sessionID string) (*PaymentOrder, error) {
	var order PaymentOrder
	err := DB.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID).First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "lookup payment order for user")
	}
	return &order, nil
}

// ListPaymentOrdersForUser returns recent payment orders for a user, newest first.
// limit is capped to 50.
func ListPaymentOrdersForUser(ctx context.Context, userID int, limit int) ([]*PaymentOrder, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var orders []*PaymentOrder
	err := DB.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id desc").
		Limit(limit).
		Find(&orders).Error
	if err != nil {
		return nil, errors.Wrap(err, "list payment orders for user")
	}
	return orders, nil
}

// HasStripeWebhookEvent reports whether eventID was already recorded as processed.
func HasStripeWebhookEvent(ctx context.Context, eventID string) (bool, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return false, nil
	}
	var n int64
	if err := DB.WithContext(ctx).Model(&StripeWebhookEvent{}).Where("event_id = ?", eventID).Count(&n).Error; err != nil {
		return false, errors.Wrap(err, "count stripe webhook event")
	}
	return n > 0, nil
}

// ClaimStripeWebhookEvent records eventID if it has not been seen before.
// Returns claimed=true when this call inserted the row (first successful record of this event).
func ClaimStripeWebhookEvent(ctx context.Context, eventID, eventType string) (claimed bool, err error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return false, errors.New("stripe event id is empty")
	}
	row := &StripeWebhookEvent{
		EventID:   eventID,
		EventType: eventType,
		CreatedAt: time.Now().UTC().UnixMilli(),
	}
	err = DB.WithContext(ctx).Create(row).Error
	if err != nil {
		if isDuplicateKeyError(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "claim stripe webhook event")
	}
	return true, nil
}

// isDuplicateKeyError reports whether err is a unique constraint violation.
func isDuplicateKeyError(err error) bool {
	return IsDuplicateKeyErrorPublic(err)
}

// IsDuplicateKeyErrorPublic reports whether err is a unique constraint violation (exported for controllers).
func IsDuplicateKeyErrorPublic(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique_violation") ||
		strings.Contains(msg, "1062")
}

// MarkPaymentOrderPaid transitions an order to paid state inside a transaction.
// Deprecated for settlement: prefer SettlePaidPaymentOrder which also credits quota.
func MarkPaymentOrderPaid(ctx context.Context, sessionID string, paidAtMs int64) (transitioned bool, order *PaymentOrder, err error) {
	return SettlePaidPaymentOrder(ctx, sessionID, paidAtMs, 0, "")
}

// paymentOrderClaimableStatuses returns states that a later authoritative paid event may settle.
func paymentOrderClaimableStatuses() []string {
	return []string{PaymentStatusPending, PaymentStatusFailed, PaymentStatusCanceled}
}

// SettlePaidPaymentOrder atomically marks a claimable order paid and credits the user quota.
// Failed and canceled states remain claimable because Stripe does not guarantee webhook ordering.
// expectedAmountCents and expectedCurrency, when non-zero/non-empty, must match the stored order.
// Returns transitioned=false when the order was already paid (idempotent no-op).
// Returns ErrPaymentOrderNotFound when no local order exists for the session.
func SettlePaidPaymentOrder(ctx context.Context, sessionID string, paidAtMs int64, expectedAmountCents int64, expectedCurrency string) (transitioned bool, order *PaymentOrder, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, nil, errors.New("session id is empty")
	}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var found PaymentOrder
		if e := tx.Where("session_id = ?", sessionID).First(&found).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return ErrPaymentOrderNotFound
			}
			return errors.Wrap(e, "load payment order in settle tx")
		}
		if found.Status == PaymentStatusPaid {
			order = &found
			return nil
		}
		if found.Status == PaymentStatusManualReview {
			order = &found
			return nil
		}
		if expectedAmountCents > 0 && found.AmountCents != expectedAmountCents {
			return markClaimableManualReview(tx, &found, paidAtMs, &order, "amount mismatch")
		}
		if expectedCurrency != "" && !strings.EqualFold(found.Currency, expectedCurrency) {
			return markClaimableManualReview(tx, &found, paidAtMs, &order, "currency mismatch")
		}

		res := tx.Model(&found).Where("status IN ?", paymentOrderClaimableStatuses()).Updates(map[string]any{
			"status":  PaymentStatusPaid,
			"paid_at": paidAtMs,
		})
		if res.Error != nil {
			return errors.Wrap(res.Error, "mark payment order paid in settle tx")
		}
		if res.RowsAffected == 0 {
			var latest PaymentOrder
			if e := tx.Where("session_id = ?", sessionID).First(&latest).Error; e != nil {
				return errors.Wrap(e, "reload payment order after empty claim")
			}
			order = &latest
			if latest.Status == PaymentStatusPaid || latest.Status == PaymentStatusManualReview {
				return nil
			}
			return errors.Errorf("payment order not claimable, status=%s", latest.Status)
		}

		if found.Quota < 0 {
			return errors.New("quota cannot be negative")
		}
		if found.Quota > 0 {
			qres := tx.Model(&User{}).Where("id = ?", found.UserId).
				Update("quota", gorm.Expr("quota + ?", found.Quota))
			if qres.Error != nil {
				return errors.Wrapf(qres.Error, "increase quota for user %d in settle tx", found.UserId)
			}
			if qres.RowsAffected != 1 {
				if e := tx.Model(&found).Where("id = ?", found.Id).Updates(map[string]any{
					"status":  PaymentStatusManualReview,
					"paid_at": paidAtMs,
				}).Error; e != nil {
					return errors.Wrap(e, "mark payment order manual_review")
				}
				found.Status = PaymentStatusManualReview
				found.PaidAt = paidAtMs
				order = &found
				transitioned = false
				return nil
			}
		}
		found.Status = PaymentStatusPaid
		found.PaidAt = paidAtMs
		transitioned = true
		order = &found
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrPaymentOrderNotFound) {
			return false, nil, ErrPaymentOrderNotFound
		}
		return false, nil, err
	}
	return transitioned, order, nil
}

// markClaimableManualReview marks a claimable order for operator review inside the settlement transaction.
func markClaimableManualReview(tx *gorm.DB, found *PaymentOrder, paidAtMs int64, orderOut **PaymentOrder, reason string) error {
	res := tx.Model(found).Where("status IN ?", paymentOrderClaimableStatuses()).Updates(map[string]any{
		"status":  PaymentStatusManualReview,
		"paid_at": paidAtMs,
	})
	if res.Error != nil {
		return errors.Wrapf(res.Error, "mark payment order manual_review after %s", reason)
	}
	if res.RowsAffected == 0 {
		var latest PaymentOrder
		if e := tx.Where("id = ?", found.Id).First(&latest).Error; e != nil {
			return errors.Wrapf(e, "reload payment order after empty manual_review claim (%s)", reason)
		}
		*orderOut = &latest
		return nil
	}
	found.Status = PaymentStatusManualReview
	found.PaidAt = paidAtMs
	*orderOut = found
	return nil
}

// MarkPaymentOrderStatus sets a terminal non-paid status on a pending order by session id.
func MarkPaymentOrderStatus(ctx context.Context, sessionID, status string) error {
	res := DB.WithContext(ctx).Model(&PaymentOrder{}).
		Where("session_id = ? AND status = ?", sessionID, PaymentStatusPending).
		Update("status", status)
	if res.Error != nil {
		return errors.Wrap(res.Error, "mark payment order status")
	}
	return nil
}
