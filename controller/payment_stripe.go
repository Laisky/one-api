package controller

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
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
}

type createStripeCheckoutResponse struct {
	URL       string `json:"url"`
	SessionID string `json:"session_id"`
}

// CreateStripeCheckout creates a Stripe Checkout Session for a freeform USD top-up.
// The fee is absorbed by the platform: the user is charged exactly AmountUSD.
// It takes a gin.Context to read the authenticated user and request body, and writes a JSON response with the checkout URL.
func CreateStripeCheckout(c *gin.Context) {
	ctx := gmw.Ctx(c)
	logger := gmw.GetLogger(c)

	if strings.TrimSpace(config.StripeSecretKey) == "" {
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

	// Derive cents first so quota stays consistent with the charged amount.
	amountCents := int64(req.AmountUSD*100 + 0.5)
	if amountCents < int64(minUSD)*100 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid amount"})
		return
	}
	quota := (amountCents * int64(config.QuotaPerUnit)) / 100

	userID := c.GetInt("id")
	base := strings.TrimRight(strings.TrimSpace(config.ServerAddress), "/")
	if base == "" {
		scheme := "https"
		if c.Request.TLS == nil && c.GetHeader("X-Forwarded-Proto") != "https" {
			scheme = "http"
		}
		base = scheme + "://" + c.Request.Host
	}

	// Look up the user's registered email so Stripe sends the receipt there.
	// Failure is non-fatal — the customer can still enter an email at Checkout.
	userEmail, _ := model.GetUserEmail(userID)
	userEmail = strings.TrimSpace(userEmail)

	// Per-request client avoids races on the package-level stripe.Key.
	sc := client.New(config.StripeSecretKey, nil)
	params := &stripe.CheckoutSessionParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
		ClientReferenceID: stripe.String(strconv.Itoa(userID)),
		SuccessURL:        stripe.String(base + "/topup/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:         stripe.String(base + "/topup/cancel"),
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
			"user_id": strconv.Itoa(userID),
			"quota":   strconv.FormatInt(quota, 10),
		},
	}
	if userEmail != "" {
		// Pre-fill the Checkout email field.
		params.CustomerEmail = stripe.String(userEmail)
		// Force Stripe to email a receipt to the user's registered address even
		// when the dashboard "email customers about successful payments" toggle
		// is off. Honored in live mode per Stripe docs.
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

	order := &model.PaymentOrder{
		UserId:      userID,
		Provider:    model.PaymentProviderStripe,
		SessionID:   session.ID,
		AmountCents: amountCents,
		Currency:    "usd",
		Quota:       quota,
		Status:      model.PaymentStatusPending,
	}
	if err := model.CreatePaymentOrder(ctx, order); err != nil {
		logger.Error("persist payment order", zap.Error(err))
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": createStripeCheckoutResponse{
			URL:       session.URL,
			SessionID: session.ID,
		},
	})
}

// StripeWebhook handles checkout.session.completed events. The route MUST receive
// the raw request body — do not interpose middleware that consumes it.
// It takes a gin.Context to parse the signed Stripe payload and writes a JSON status response.
func StripeWebhook(c *gin.Context) {
	ctx := gmw.Ctx(c)
	logger := gmw.GetLogger(c)

	secret := strings.TrimSpace(config.StripeWebhookSecret)
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret not configured"})
		return
	}

	// Cap body size to mitigate OOM on a public webhook endpoint (Stripe events are small).
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

	if event.Type != "checkout.session.completed" && event.Type != "checkout.session.async_payment_succeeded" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	sessionID, _ := event.Data.Object["id"].(string)
	paymentStatus, _ := event.Data.Object["payment_status"].(string)
	if sessionID == "" || paymentStatus != "paid" {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	// Atomic settle: mark paid + credit quota in one DB transaction (idempotent).
	transitioned, order, err := model.SettlePaidPaymentOrder(ctx, sessionID, time.Now().UnixMilli())
	if err != nil {
		logger.Error("settle payment order", zap.Error(err), zap.String("session_id", sessionID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if order == nil {
		logger.Warn("stripe webhook for unknown session", zap.String("session_id", sessionID))
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}
	if !transitioned {
		c.JSON(http.StatusOK, gin.H{"received": true})
		return
	}

	remark := fmt.Sprintf("Stripe top-up $%.2f (%s)", float64(order.AmountCents)/100, common.LogQuota(order.Quota))
	model.RecordTopupLog(ctx, order.UserId, remark, int(order.Quota))

	c.JSON(http.StatusOK, gin.H{"received": true})
}
