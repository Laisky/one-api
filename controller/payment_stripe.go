package controller

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	stripe "github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/client"
	stripewebhook "github.com/stripe/stripe-go/v82/webhook"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

type createStripeCheckoutRequest struct {
	AmountUSD float64 `json:"amount_usd"`
	RequestID string  `json:"request_id"`
}

type createStripeCheckoutResponse struct {
	URL       string `json:"url"`
	SessionID string `json:"session_id"`
	RequestID string `json:"request_id"`
}

// StripeReady reports whether Checkout and webhook settlement are fully configured.
// It requires both the Stripe secret key and webhook secret, plus a trusted public HTTPS base URL.
func StripeReady() bool {
	if strings.TrimSpace(config.StripeSecretKey) == "" {
		return false
	}
	if strings.TrimSpace(config.StripeWebhookSecret) == "" {
		return false
	}
	_, err := stripePublicBaseURL(true)
	return err == nil
}

// stripePublicBaseURL returns the trusted public origin used for Checkout return URLs.
// When requireHTTPS is true, http:// origins are rejected (live Checkout must not bounce to HTTP).
func stripePublicBaseURL(requireHTTPS bool) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(config.StripePublicBaseURL), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(config.ServerAddress), "/")
	}
	if base == "" {
		return "", errors.New("stripe public base URL is not configured (set STRIPE_PUBLIC_BASE_URL or ServerAddress)")
	}
	lower := strings.ToLower(base)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		return "", errors.New("stripe public base URL must include http:// or https://")
	}
	if requireHTTPS && !strings.HasPrefix(lower, "https://") {
		// Allow http only for explicit local/dev secrets (sk_test) without live keys.
		if strings.HasPrefix(strings.TrimSpace(config.StripeSecretKey), "sk_live") {
			return "", errors.New("live Stripe keys require an https public base URL")
		}
	}
	return base, nil
}

// CreateStripeCheckout creates a Stripe Checkout Session for a freeform USD top-up.
// The fee is absorbed by the platform: the user is charged exactly AmountUSD.
// It takes a gin.Context to read the authenticated user and request body, and writes a JSON response with the checkout URL.
func CreateStripeCheckout(c *gin.Context) {
	ctx := gmw.Ctx(c)
	logger := gmw.GetLogger(c)

	if !StripeReady() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Stripe is not configured"})
		return
	}

	var req createStripeCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	minUSD := config.MinTopUpUSD
	if minUSD < 1 {
		minUSD = 1
	}
	if req.AmountUSD < float64(minUSD) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("minimum top-up amount is $%d", minUSD),
		})
		return
	}
	if req.AmountUSD > 100000 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "amount too large"})
		return
	}

	amountCents := int64(req.AmountUSD*100 + 0.5)
	if amountCents < int64(minUSD)*100 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid amount"})
		return
	}
	quota := (amountCents * int64(config.QuotaPerUnit)) / 100

	userID := c.GetInt("id")
	base, err := stripePublicBaseURL(true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID, err = model.NewPaymentRequestID()
		if err != nil {
			logger.Error("generate payment request id", zap.Error(err))
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "failed to create order"})
			return
		}
	}
	if len(requestID) > 64 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "request_id too long"})
		return
	}

	// Local order first so retries can reuse the same request_id / Idempotency-Key.
	placeholderSession := "pending:" + requestID
	order := &model.PaymentOrder{
		UserId:      userID,
		Provider:    model.PaymentProviderStripe,
		RequestID:   requestID,
		SessionID:   placeholderSession,
		AmountCents: amountCents,
		Currency:    "usd",
		Quota:       quota,
		Status:      model.PaymentStatusPending,
	}
	if err := model.CreatePaymentOrder(ctx, order); err != nil {
		// Idempotent retry: reuse the existing row for this request_id.
		existing, lookErr := model.GetPaymentOrderByRequestID(ctx, userID, requestID)
		if lookErr != nil {
			logger.Error("lookup payment order by request id", zap.Error(lookErr))
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		if existing == nil {
			logger.Error("persist payment order", zap.Error(err))
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		order = existing
		if !strings.HasPrefix(existing.SessionID, "pending:") && existing.SessionID != "" {
			// Session already bound; Stripe Idempotency-Key will return the same session URL on New below.
		}
	}

	userEmail, emailErr := model.GetUserEmail(userID)
	if emailErr != nil {
		logger.Warn("lookup user email for stripe receipt", zap.Error(emailErr), zap.Int("user_id", userID))
	}
	userEmail = strings.TrimSpace(userEmail)

	sc := client.New(config.StripeSecretKey, nil)
	params := &stripe.CheckoutSessionParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
		ClientReferenceID: stripe.String(strconv.Itoa(userID)),
		SuccessURL:        stripe.String(base + "/topup?stripe=success&session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:         stripe.String(base + "/topup?stripe=cancel"),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Quantity: stripe.Int64(1),
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String(string(stripe.CurrencyUSD)),
					UnitAmount: stripe.Int64(amountCents),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String(fmt.Sprintf("Quota top-up: $%.2f", float64(amountCents)/100)),
					},
				},
			},
		},
		Metadata: map[string]string{
			"user_id":    strconv.Itoa(userID),
			"quota":      strconv.FormatInt(quota, 10),
			"request_id": requestID,
			"order_id":   strconv.Itoa(order.Id),
		},
	}
	params.SetIdempotencyKey(requestID)
	if userEmail != "" {
		params.CustomerEmail = stripe.String(userEmail)
		params.PaymentIntentData = &stripe.CheckoutSessionPaymentIntentDataParams{
			ReceiptEmail: stripe.String(userEmail),
		}
	}

	session, err := sc.CheckoutSessions.New(params)
	if err != nil {
		logger.Error("create stripe checkout session", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	if err := model.BindPaymentOrderSession(ctx, order.Id, session.ID); err != nil {
		logger.Error("bind payment order session", zap.Error(err), zap.Int("order_id", order.Id), zap.String("session_id", session.ID))
		// Session already exists at Stripe; surface URL so the user can still pay.
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": createStripeCheckoutResponse{
			URL:       session.URL,
			SessionID: session.ID,
			RequestID: requestID,
		},
	})
}

// GetStripePaymentOrder returns the authenticated user's payment order for a Checkout session.
func GetStripePaymentOrder(c *gin.Context) {
	ctx := gmw.Ctx(c)
	userID := c.GetInt("id")
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "session_id required"})
		return
	}
	order, err := model.GetPaymentOrderBySessionForUser(ctx, userID, sessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if order == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "order not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": order})
}

// ListStripePaymentOrders returns recent Stripe payment orders for the authenticated user.
func ListStripePaymentOrders(c *gin.Context) {
	ctx := gmw.Ctx(c)
	userID := c.GetInt("id")
	orders, err := model.ListPaymentOrdersForUser(ctx, userID, 20)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": orders})
}

// StripeWebhook handles checkout.session.* payment events. The route MUST receive
// the raw request body — do not interpose middleware that consumes it.
func StripeWebhook(c *gin.Context) {
	ctx := gmw.Ctx(c)
	logger := gmw.GetLogger(c)

	secret := strings.TrimSpace(config.StripeWebhookSecret)
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret not configured"})
		return
	}

	const maxStripeWebhookBody = 1 << 20 // 1 MiB
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, maxStripeWebhookBody))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(payload) >= maxStripeWebhookBody {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "webhook payload too large"})
		return
	}

	event, err := stripewebhook.ConstructEvent(payload, c.GetHeader("Stripe-Signature"), secret)
	if err != nil {
		logger.Warn("stripe webhook signature verification failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "signature verification failed"})
		return
	}

	// Skip work when this event id was already successfully processed.
	seen, seenErr := model.HasStripeWebhookEvent(ctx, event.ID)
	if seenErr != nil {
		logger.Error("check stripe webhook event", zap.Error(seenErr), zap.String("event_id", event.ID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event lookup failed"})
		return
	}
	if seen {
		c.JSON(http.StatusOK, gin.H{"received": true, "duplicate": true})
		return
	}

	// Process first; claim only after success so Stripe can safely retry 5xx failures.
	// Settlement itself is idempotent via pending→paid; claim only records successful delivery.
	var finished bool
	switch event.Type {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		finished = handleCheckoutPaid(c, event)
	case "checkout.session.expired", "checkout.session.async_payment_failed":
		finished = handleCheckoutTerminal(c, event)
	default:
		c.JSON(http.StatusOK, gin.H{"received": true})
		finished = true
	}
	if !finished {
		return
	}
	if _, claimErr := model.ClaimStripeWebhookEvent(ctx, event.ID, string(event.Type)); claimErr != nil {
		// Settlement already succeeded; log only. Duplicate claim is fine under concurrency.
		if !model.IsDuplicateKeyErrorPublic(claimErr) {
			logger.Warn("record processed stripe event", zap.Error(claimErr), zap.String("event_id", event.ID))
		}
	}
}

// handleCheckoutPaid settles a paid Checkout session and credits quota.
// It returns true when the HTTP response is terminal for Stripe (2xx) and the event may be claimed.
// It returns false when a 5xx was written and Stripe should retry with the same event id.
func handleCheckoutPaid(c *gin.Context, event stripe.Event) bool {
	ctx := gmw.Ctx(c)
	logger := gmw.GetLogger(c)
	sessionID, _ := event.Data.Object["id"].(string)
	paymentStatus, _ := event.Data.Object["payment_status"].(string)
	currency, _ := event.Data.Object["currency"].(string)
	var amountTotal int64
	switch v := event.Data.Object["amount_total"].(type) {
	case float64:
		amountTotal = int64(v)
	case int64:
		amountTotal = v
	case int:
		amountTotal = int64(v)
	}
	if sessionID == "" || paymentStatus != "paid" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return true
	}

	transitioned, order, err := model.SettlePaidPaymentOrder(ctx, sessionID, time.Now().UnixMilli(), amountTotal, currency)
	if err != nil {
		if errors.Is(err, model.ErrPaymentOrderNotFound) {
			logger.Warn("stripe webhook for unknown session", zap.String("session_id", sessionID), zap.String("event_id", event.ID))
			// Acknowledge so Stripe stops retrying; operator can reconcile via dashboard.
			c.JSON(http.StatusOK, gin.H{"received": true, "unknown_session": true})
			return true
		}
		logger.Error("settle payment order", zap.Error(err), zap.String("session_id", sessionID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if order != nil && order.Status == model.PaymentStatusManualReview {
		logger.Error("payment needs manual review", zap.String("session_id", sessionID), zap.Int("user_id", order.UserId))
		c.JSON(http.StatusOK, gin.H{"received": true, "manual_review": true})
		return true
	}
	if order == nil || !transitioned {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return true
	}

	// Best-effort cache refresh and audit log (must not undo settlement).
	if cacheErr := model.CacheUpdateUserQuota(ctx, order.UserId); cacheErr != nil {
		logger.Warn("refresh user quota cache after stripe top-up", zap.Error(cacheErr), zap.Int("user_id", order.UserId))
	}
	remark := fmt.Sprintf("Stripe top-up $%.2f (%s)", float64(order.AmountCents)/100, common.LogQuota(order.Quota))
	model.RecordTopupLog(ctx, order.UserId, remark, int(order.Quota))

	c.JSON(http.StatusOK, gin.H{"received": true})
	return true
}

// handleCheckoutTerminal marks pending orders expired/failed without crediting quota.
// It returns true when the response is terminal for Stripe and the event may be claimed.
func handleCheckoutTerminal(c *gin.Context, event stripe.Event) bool {
	ctx := gmw.Ctx(c)
	logger := gmw.GetLogger(c)
	sessionID, _ := event.Data.Object["id"].(string)
	if sessionID == "" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return true
	}
	status := model.PaymentStatusCanceled
	if event.Type == "checkout.session.async_payment_failed" {
		status = model.PaymentStatusFailed
	}
	if err := model.MarkPaymentOrderStatus(ctx, sessionID, status); err != nil {
		logger.Error("mark payment terminal status", zap.Error(err), zap.String("session_id", sessionID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	c.JSON(http.StatusOK, gin.H{"received": true})
	return true
}
