package controller

import (
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
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

const (
	maxStripeTopUpUSD      = int64(100_000)
	maxStripeWebhookBytes  = int64(64 << 10)
	stripePaymentCurrency  = "usd"
	stripePendingSessionID = "pending:stripe:"
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

type stripeCheckoutSessionClient interface {
	New(params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error)
	Expire(id string, params *stripe.CheckoutSessionExpireParams) (*stripe.CheckoutSession, error)
}

var newStripeCheckoutSessionClient = func(secretKey string) stripeCheckoutSessionClient {
	return client.New(secretKey, nil).CheckoutSessions
}

// StripeReady reports whether Checkout and webhook settlement are fully configured.
func StripeReady() bool {
	if strings.TrimSpace(config.StripeSecretKey) == "" || strings.TrimSpace(config.StripeWebhookSecret) == "" {
		return false
	}
	_, err := stripePublicBaseURL(true)
	return err == nil
}

// stripePublicBaseURL returns a normalized origin used for Checkout return URLs.
// HTTPS is required except for loopback development with an sk_test key.
func stripePublicBaseURL(requireHTTPS bool) (string, error) {
	raw := strings.TrimSpace(config.StripePublicBaseURL)
	if raw == "" {
		raw = strings.TrimSpace(config.ServerAddress)
	}
	if raw == "" {
		return "", errors.New("stripe public base URL is not configured")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.Wrap(err, "parse stripe public base URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", errors.New("stripe public base URL must be an absolute HTTP(S) origin")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("stripe public base URL must not contain credentials, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("stripe public base URL must not contain a path")
	}

	if requireHTTPS && parsed.Scheme != "https" {
		isTestKey := strings.HasPrefix(strings.TrimSpace(config.StripeSecretKey), "sk_test_")
		if !isTestKey || !isLoopbackHost(parsed.Hostname()) {
			return "", errors.New("stripe public base URL must use HTTPS")
		}
	}

	parsed.Path = ""
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func stripeAmountCents(amountUSD float64, minUSD int) (int64, error) {
	if math.IsNaN(amountUSD) || math.IsInf(amountUSD, 0) || amountUSD <= 0 {
		return 0, errors.New("amount must be a positive finite number")
	}
	if minUSD < 1 {
		minUSD = 1
	}

	scaled := amountUSD * 100
	rounded := math.Round(scaled)
	if math.Abs(scaled-rounded) > 1e-7 {
		return 0, errors.New("amount must have at most two decimal places")
	}
	if rounded > float64(maxStripeTopUpUSD*100) {
		return 0, errors.New("amount too large")
	}
	cents := int64(rounded)
	if cents < int64(minUSD)*100 {
		return 0, errors.Errorf("minimum top-up amount is $%d", minUSD)
	}
	return cents, nil
}

func validStripeRequestID(requestID string) bool {
	if requestID == "" || len(requestID) > 64 {
		return false
	}
	for i := range requestID {
		c := requestID[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func stripeIdempotencyKey(userID int, requestID string) string {
	return fmt.Sprintf("one-api:stripe:checkout:v1:%d:%s", userID, requestID)
}

func stripePendingSession(userID int, requestID string) string {
	return fmt.Sprintf("%s%d:%s", stripePendingSessionID, userID, requestID)
}

func paymentOrderMatches(order *model.PaymentOrder, amountCents, quota int64) bool {
	return order != nil &&
		order.Provider == model.PaymentProviderStripe &&
		order.AmountCents == amountCents &&
		strings.EqualFold(order.Currency, stripePaymentCurrency) &&
		order.Quota == quota &&
		order.Status == model.PaymentStatusPending
}

// CreateStripeCheckout creates a Stripe Checkout Session for a freeform USD top-up.
// The local order is durable before a payment URL is returned to the caller.
func CreateStripeCheckout(c *gin.Context) {
	ctx := gmw.Ctx(c)
	logger := gmw.GetLogger(c)

	if !StripeReady() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Stripe is not configured"})
		return
	}

	var req createStripeCheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid checkout request"})
		return
	}

	amountCents, err := stripeAmountCents(req.AmountUSD, config.MinTopUpUSD)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	quota := (amountCents * int64(config.QuotaPerUnit)) / 100
	if quota <= 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "top-up quota is not configured"})
		return
	}

	userID := c.GetInt("id")
	base, err := stripePublicBaseURL(true)
	if err != nil {
		logger.Error("validate Stripe public base URL", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Stripe is not configured"})
		return
	}

	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID, err = model.NewPaymentRequestID()
		if err != nil {
			logger.Error("generate payment request id", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create order"})
			return
		}
	}
	if !validStripeRequestID(requestID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "request_id must contain only letters, digits, '-' or '_' and be at most 64 characters"})
		return
	}

	order := &model.PaymentOrder{
		UserId:      userID,
		Provider:    model.PaymentProviderStripe,
		RequestID:   requestID,
		SessionID:   stripePendingSession(userID, requestID),
		AmountCents: amountCents,
		Currency:    stripePaymentCurrency,
		Quota:       quota,
		Status:      model.PaymentStatusPending,
	}
	if err := model.CreatePaymentOrder(ctx, order); err != nil {
		existing, lookupErr := model.GetPaymentOrderByRequestID(ctx, userID, requestID)
		if lookupErr != nil {
			logger.Error("lookup payment order by request id", zap.Error(lookupErr), zap.Int("user_id", userID))
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create order"})
			return
		}
		if !paymentOrderMatches(existing, amountCents, quota) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "request_id was already used for a different or completed order"})
			return
		}
		order = existing
	}

	userEmail, emailErr := model.GetUserEmail(userID)
	if emailErr != nil {
		logger.Warn("lookup user email for Stripe receipt", zap.Error(emailErr), zap.Int("user_id", userID))
	}
	userEmail = strings.TrimSpace(userEmail)

	// Stripe is a third party: hand it the external UUID, never the internal
	// integer id. The webhook settles by session id and never reads these
	// values, so they are informational only.
	userRef, uuidErr := model.GetUserUUIDByID(userID)
	if uuidErr != nil || userRef == "" {
		logger.Warn("lookup user uuid for Stripe metadata", zap.Error(uuidErr), zap.Int("user_id", userID))
		userRef = strconv.Itoa(userID)
	}

	params := &stripe.CheckoutSessionParams{
		Mode:              stripe.String(string(stripe.CheckoutSessionModePayment)),
		ClientReferenceID: stripe.String(userRef),
		SuccessURL:        stripe.String(base + "/topup?stripe=success&session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:         stripe.String(base + "/topup?stripe=cancel"),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String(string(stripe.CurrencyUSD)),
				UnitAmount: stripe.Int64(amountCents),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String(fmt.Sprintf("Quota top-up: $%.2f", float64(amountCents)/100)),
				},
			},
		}},
		Metadata: map[string]string{
			"user_id":    userRef,
			"quota":      strconv.FormatInt(quota, 10),
			"request_id": requestID,
			"order_id":   strconv.Itoa(order.Id),
		},
	}
	params.SetIdempotencyKey(stripeIdempotencyKey(userID, requestID))
	if userEmail != "" {
		params.CustomerEmail = stripe.String(userEmail)
		params.PaymentIntentData = &stripe.CheckoutSessionPaymentIntentDataParams{ReceiptEmail: stripe.String(userEmail)}
	}

	stripeClient := newStripeCheckoutSessionClient(config.StripeSecretKey)
	session, err := stripeClient.New(params)
	if err != nil {
		logger.Error("create Stripe Checkout Session", zap.Error(err), zap.Int("order_id", order.Id))
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "failed to create Stripe Checkout Session"})
		return
	}
	if session == nil || strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.URL) == "" {
		logger.Error("Stripe returned an incomplete Checkout Session", zap.Int("order_id", order.Id))
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "failed to create Stripe Checkout Session"})
		return
	}

	if err := model.BindPaymentOrderSession(ctx, order.Id, session.ID); err != nil {
		logger.Error("bind payment order session", zap.Error(err), zap.Int("order_id", order.Id), zap.String("session_id", session.ID))
		if _, expireErr := stripeClient.Expire(session.ID, &stripe.CheckoutSessionExpireParams{}); expireErr != nil {
			logger.Error("expire unbound Stripe Checkout Session", zap.Error(expireErr), zap.String("session_id", session.ID))
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to initialize checkout"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    createStripeCheckoutResponse{URL: session.URL, SessionID: session.ID, RequestID: requestID},
	})
}

// GetStripePaymentOrder returns the authenticated user's payment order for a Checkout session.
func GetStripePaymentOrder(c *gin.Context) {
	ctx := gmw.Ctx(c)
	userID := c.GetInt("id")
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "session_id required"})
		return
	}
	order, err := model.GetPaymentOrderBySessionForUser(ctx, userID, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to load order"})
		return
	}
	if order == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "order not found"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to load orders"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": orders})
}

// StripeWebhook handles checkout.session.* payment events. The route must receive the raw request body.
func StripeWebhook(c *gin.Context) {
	ctx := gmw.Ctx(c)
	logger := gmw.GetLogger(c)

	secret := strings.TrimSpace(config.StripeWebhookSecret)
	if secret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhook secret not configured"})
		return
	}

	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, maxStripeWebhookBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook payload"})
		return
	}
	if int64(len(payload)) > maxStripeWebhookBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "webhook payload too large"})
		return
	}

	event, err := stripewebhook.ConstructEventWithOptions(
		payload,
		c.GetHeader("Stripe-Signature"),
		secret,
		stripewebhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true},
	)
	if err != nil {
		logger.Warn("Stripe webhook signature verification failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "signature verification failed"})
		return
	}

	seen, seenErr := model.HasStripeWebhookEvent(ctx, event.ID)
	if seenErr != nil {
		logger.Error("check Stripe webhook event", zap.Error(seenErr), zap.String("event_id", event.ID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "event lookup failed"})
		return
	}
	if seen {
		c.JSON(http.StatusOK, gin.H{"received": true, "duplicate": true})
		return
	}

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
	if _, claimErr := model.ClaimStripeWebhookEvent(ctx, event.ID, string(event.Type)); claimErr != nil && !model.IsDuplicateKeyErrorPublic(claimErr) {
		logger.Warn("record processed Stripe event", zap.Error(claimErr), zap.String("event_id", event.ID))
	}
}

// handleCheckoutPaid settles a paid Checkout session and credits quota.
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
			logger.Error("Stripe webhook references an unknown session", zap.String("session_id", sessionID), zap.String("event_id", event.ID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "payment order not found"})
			return false
		}
		logger.Error("settle payment order", zap.Error(err), zap.String("session_id", sessionID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "settlement failed"})
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

	if cacheErr := model.CacheUpdateUserQuota(ctx, order.UserId); cacheErr != nil {
		logger.Warn("refresh user quota cache after Stripe top-up", zap.Error(cacheErr), zap.Int("user_id", order.UserId))
	}
	remark := fmt.Sprintf("Stripe top-up $%.2f (%s)", float64(order.AmountCents)/100, common.LogQuota(order.Quota))
	model.RecordTopupLog(ctx, order.UserId, remark, int(order.Quota))

	c.JSON(http.StatusOK, gin.H{"received": true})
	return true
}

// handleCheckoutTerminal marks pending orders expired or failed without crediting quota.
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "status update failed"})
		return false
	}
	c.JSON(http.StatusOK, gin.H{"received": true})
	return true
}
