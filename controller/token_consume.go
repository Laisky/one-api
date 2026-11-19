package controller

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/model"
)

// ConsumePhase defines the allowed lifecycle phases for external billing events.
type ConsumePhase string

const (
	// ConsumePhasePre marks the initial reservation stage. Quota is held while
	// the upstream job executes, and must later be reconciled with a post event.
	ConsumePhasePre ConsumePhase = "pre"
	// ConsumePhasePost finalizes a prior reservation. The caller supplies the
	// authoritative usage total, allowing One-API to refund or charge any delta.
	ConsumePhasePost ConsumePhase = "post"
	// ConsumePhaseCancel releases a pending reservation without consuming quota,
	// typically used when the upstream job is aborted or fails before completion.
	ConsumePhaseCancel ConsumePhase = "cancel"
	// ConsumePhaseSingle performs reservation and reconciliation in a single
	// request, preserving backward compatibility with legacy integrations.
	ConsumePhaseSingle ConsumePhase = "single"
)

// String returns the string representation of the consume phase.
func (c ConsumePhase) String() string {
	return string(c)
}

type consumeTokenRequest struct {
	// AddUsedQuota represents the amount of quota to reserve or finalize (in quota units).
	AddUsedQuota uint64 `json:"add_used_quota,omitempty" gorm:"-"`
	// AddReason explains the source of the external billing event.
	AddReason string `json:"add_reason" gorm:"-"`
	// ElapsedTimeMs optionally records the upstream processing latency in milliseconds.
	ElapsedTimeMs *int64 `json:"elapsed_time_ms,omitempty" gorm:"-"`
	// Phase indicates the lifecycle stage: pre, post, cancel, or empty for immediate consumption. Optional for backward compatibility.
	Phase *ConsumePhase `json:"phase,omitempty" gorm:"-"`
	// TransactionID uniquely identifies the external billing transaction. Optional for backward compatibility.
	TransactionID *string `json:"transaction_id,omitempty" gorm:"-"`
	// FinalUsedQuota provides the reconciled quota during the post phase. When omitted, add_used_quota is used.
	FinalUsedQuota *uint64 `json:"final_used_quota,omitempty" gorm:"-"`
	// TimeoutSeconds allows callers to customize the auto-confirm window for pre holds.
	TimeoutSeconds *int64 `json:"timeout_seconds,omitempty" gorm:"-"`
}

// ConsumeToken processes external billing events, supporting pre/post confirmation,
// cancellation, and immediate consumption flows.
func ConsumeToken(c *gin.Context) {
	ctx := gmw.Ctx(c)
	userID := c.GetInt(ctxkey.Id)
	tokenID := c.GetInt(ctxkey.TokenId)

	if err := autoConfirmExpiredTokenTransactions(ctx, c, tokenID); err != nil {
		helper.RespondError(c, err)
		return
	}

	req := new(consumeTokenRequest)
	if err := c.ShouldBindJSON(req); err != nil {
		// Malformed request body: the caller sent JSON this endpoint cannot bind.
		helper.RespondError(c, errkind.InvalidRequestErr(err))
		return
	}

	req.AddReason = strings.TrimSpace(req.AddReason)
	if req.AddReason == "" {
		helper.RespondError(c, errkind.InvalidRequestErr(errors.New("add_reason cannot be empty")))
		return
	}

	phaseValue := ""
	if req.Phase != nil {
		phaseValue = strings.ToLower(strings.TrimSpace(req.Phase.String()))
	}

	phase := ConsumePhase(phaseValue)
	if phase == "" {
		phase = ConsumePhaseSingle
	}

	cleanToken, err := model.GetTokenByIds(tokenID, userID)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	if err := validateTokenForConsumption(cleanToken); err != nil {
		helper.RespondError(c, err)
		return
	}

	traceID := ""
	if tid, err := gmw.TraceID(c); err == nil {
		traceID = tid.String()
	}
	requestID := c.GetString(helper.RequestIdKey)

	var (
		transaction  *model.TokenTransaction
		updatedToken *model.Token
	)

	switch phase {
	case ConsumePhasePre:
		transaction, updatedToken, err = processPreConsume(ctx, c, cleanToken, userID, req, requestID, traceID)
	case ConsumePhasePost:
		transaction, updatedToken, err = processPostConsume(ctx, c, cleanToken, userID, req, nil)
	case ConsumePhaseCancel:
		transaction, updatedToken, err = processCancelConsume(ctx, c, cleanToken, userID, req)
	case ConsumePhaseSingle:
		transaction, updatedToken, err = processImmediateConsume(ctx, c, cleanToken, userID, req, requestID, traceID)
	default:
		helper.RespondError(c, errkind.InvalidRequestErr(errors.Errorf("unsupported phase: %s", phase)))
		return
	}

	if err != nil {
		helper.RespondError(c, err)
		return
	}

	// ToResponse returns the zero object for a nil receiver, so guard the nil
	// case to preserve the historical `"data": null` when no token was updated.
	var tokenData any
	if updatedToken != nil {
		tokenData = updatedToken.ToResponse()
	}
	response := gin.H{
		"success": true,
		"message": "",
		"data":    tokenData,
	}
	if transaction != nil {
		response["transaction"] = buildTransactionResponse(transaction)
	}

	c.JSON(http.StatusOK, response)
}

// processPreConsume reserves quota for a future post confirmation and records a pending transaction.
func processPreConsume(ctx context.Context, _ *gin.Context, token *model.Token, userID int, req *consumeTokenRequest, requestID string, traceID string) (*model.TokenTransaction, *model.Token, error) {
	if req.AddUsedQuota == 0 {
		return nil, nil, errors.New("add_used_quota must be greater than 0 for pre phase")
	}

	preQuota, err := quotaToInt64(req.AddUsedQuota, "add_used_quota")
	if err != nil {
		return nil, nil, errors.Wrap(err, "convert add_used_quota to int64")
	}

	transactionID, err := generateTransactionID(ctx, token.Id)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generate transaction id")
	}
	req.TransactionID = &transactionID

	timeoutSeconds := normalizeTimeoutSeconds(req.TimeoutSeconds)
	expiresAt := helper.GetTimestamp() + timeoutSeconds

	if err = model.PreConsumeTokenQuota(ctx, token.Id, preQuota); err != nil {
		return nil, nil, errors.Wrap(err, "pre-consume token quota")
	}

	logEntry := &model.Log{
		UserId:    userID,
		UserUUID:  token.UserUUID,
		ModelName: req.AddReason,
		TokenName: token.Name,
		TokenUUID: &token.UUID,
		Quota:     clampQuotaToInt(preQuota),
		Content:   buildPreConsumeLogContent(req.AddReason, preQuota, transactionID, timeoutSeconds),
		RequestId: requestID,
		TraceId:   traceID,
	}

	// External /api/token/consume rows are tool-typed so they appear in the
	// dashboard's tool charts instead of model usage charts.
	model.RecordToolLog(ctx, logEntry)

	transaction := &model.TokenTransaction{
		TransactionID: transactionID,
		TokenId:       token.Id,
		TokenUUID:     &token.UUID,
		UserId:        userID,
		UserUUID:      token.UserUUID,
		Status:        model.TokenTransactionStatusPending,
		PreQuota:      preQuota,
		Reason:        req.AddReason,
		RequestId:     requestID,
		TraceId:       traceID,
		ExpiresAt:     expiresAt,
		LogId:         &logEntry.Id,
	}
	if logEntry.UUID != "" {
		transaction.LogUUID = &logEntry.UUID
	}

	if req.ElapsedTimeMs != nil && *req.ElapsedTimeMs > 0 {
		elapsed := *req.ElapsedTimeMs
		transaction.ElapsedTimeMs = &elapsed
	}

	if err = model.CreateTokenTransaction(ctx, transaction); err != nil {
		_ = model.PostConsumeTokenQuota(ctx, token.Id, -preQuota)
		if logEntry.Id > 0 {
			_ = model.UpdateConsumeLogByID(ctx, logEntry.Id, map[string]any{
				"quota":   0,
				"content": fmt.Sprintf("External (%s) pre-consume aborted (transaction %s)", req.AddReason, transactionID),
			})
		}
		return nil, nil, errors.Wrap(err, "create token transaction")
	}

	updatedToken, err := model.GetTokenByIds(token.Id, userID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get token by ids after pre-consume")
	}

	return transaction, updatedToken, nil
}

// processPostConsume reconciles a pre-consumed transaction using the final quota value.
func processPostConsume(ctx context.Context, c *gin.Context, token *model.Token, userID int, req *consumeTokenRequest, existingTxn *model.TokenTransaction) (*model.TokenTransaction, *model.Token, error) {
	var transactionID string
	if req.TransactionID != nil {
		transactionID = strings.TrimSpace(*req.TransactionID)
	}
	if transactionID == "" {
		return nil, nil, errors.New("transaction_id is required for post phase")
	}

	var err error
	if existingTxn == nil {
		existingTxn, err = model.GetTokenTransactionByTokenAndID(ctx, token.Id, transactionID)
		if err != nil {
			return nil, nil, errors.Wrap(err, "get token transaction for post-consume")
		}
	}

	if existingTxn.Status != model.TokenTransactionStatusPending {
		return nil, nil, errors.Errorf("transaction %s is already %s", transactionID, model.TokenTransactionStatusString(existingTxn.Status))
	}

	finalQuotaValue := req.FinalUsedQuota
	if finalQuotaValue == nil {
		if req.AddUsedQuota == 0 {
			return nil, nil, errors.New("final_used_quota or add_used_quota must be provided for post phase")
		}
		finalQuotaValue = &req.AddUsedQuota
	}

	finalQuota, err := quotaToInt64(*finalQuotaValue, "final_used_quota")
	if err != nil {
		return nil, nil, errors.Wrap(err, "convert final_used_quota to int64")
	}

	reservationExpiresAt := existingTxn.ExpiresAt
	existingTxn, err = model.FinalizePendingTokenTransaction(ctx, token.Id, existingTxn.Id, model.TokenTransactionFinalization{
		Status:        model.TokenTransactionStatusConfirmed,
		FinalQuota:    finalQuota,
		At:            helper.GetTimestamp(),
		Reason:        &req.AddReason,
		ElapsedTimeMs: req.ElapsedTimeMs,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "finalize post-consume transaction")
	}

	if existingTxn.LogId != nil {
		logContent := buildPostConsumeLogContent(req.AddReason, existingTxn.PreQuota, finalQuota, transactionID)
		logUpdates := map[string]any{
			"quota":   clampQuotaToInt(finalQuota),
			"content": logContent,
		}
		if req.ElapsedTimeMs != nil && *req.ElapsedTimeMs > 0 {
			logUpdates["elapsed_time"] = *req.ElapsedTimeMs
		}
		if err = model.UpdateConsumeLogByID(ctx, *existingTxn.LogId, logUpdates); err != nil {
			gmw.GetLogger(c).Error("failed to update consume log after post confirmation",
				zap.Error(err),
				zap.String("transaction_id", transactionID),
			)
		}
	}

	updatedToken, err := model.GetTokenByIds(token.Id, userID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get token by ids after post-consume")
	}

	// Preserve the historical response deadline without reopening the persisted
	// terminal transaction, whose expires_at stays zero after atomic finalization.
	existingTxn.ExpiresAt = reservationExpiresAt
	return existingTxn, updatedToken, nil
}

// processCancelConsume cancels a pending transaction and refunds the reserved quota.
func processCancelConsume(ctx context.Context, c *gin.Context, token *model.Token, userID int, req *consumeTokenRequest) (*model.TokenTransaction, *model.Token, error) {
	var transactionID string
	if req.TransactionID != nil {
		transactionID = strings.TrimSpace(*req.TransactionID)
	}
	if transactionID == "" {
		return nil, nil, errors.New("transaction_id is required for cancel phase")
	}

	txn, err := model.GetTokenTransactionByTokenAndID(ctx, token.Id, transactionID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get token transaction for cancel")
	}

	if txn.Status != model.TokenTransactionStatusPending {
		return nil, nil, errors.Errorf("transaction %s cannot be canceled because it is %s", transactionID, model.TokenTransactionStatusString(txn.Status))
	}

	reservationExpiresAt := txn.ExpiresAt
	txn, err = model.FinalizePendingTokenTransaction(ctx, token.Id, txn.Id, model.TokenTransactionFinalization{
		Status: model.TokenTransactionStatusCanceled,
		At:     helper.GetTimestamp(),
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "finalize canceled transaction")
	}

	if txn.LogId != nil {
		logUpdates := map[string]any{
			"quota":   0,
			"content": buildCancelConsumeLogContent(txn.Reason, txn.PreQuota, transactionID),
		}
		if txn.ElapsedTimeMs != nil {
			logUpdates["elapsed_time"] = *txn.ElapsedTimeMs
		}
		if err = model.UpdateConsumeLogByID(ctx, *txn.LogId, logUpdates); err != nil {
			gmw.GetLogger(c).Error("failed to update consume log after cancel",
				zap.Error(err),
				zap.String("transaction_id", transactionID),
			)
		}
	}

	updatedToken, err := model.GetTokenByIds(token.Id, userID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get token by ids after cancel")
	}

	// Preserve the historical response deadline without reopening the persisted
	// terminal transaction, whose expires_at stays zero after atomic finalization.
	txn.ExpiresAt = reservationExpiresAt
	return txn, updatedToken, nil
}

// processImmediateConsume performs a pre and post flow back-to-back for legacy single-phase clients.
func processImmediateConsume(ctx context.Context, c *gin.Context, token *model.Token, userID int, req *consumeTokenRequest, requestID string, traceID string) (*model.TokenTransaction, *model.Token, error) {
	if req.AddUsedQuota == 0 {
		return processZeroQuotaImmediateConsume(ctx, token, userID, req, requestID, traceID)
	}

	var transactionID string
	if req.TransactionID != nil {
		transactionID = strings.TrimSpace(*req.TransactionID)
	}
	if transactionID == "" {
		uuid := random.GetUUID()
		req.TransactionID = &uuid
	}

	transaction, updatedToken, err := processPreConsume(ctx, c, token, userID, req, requestID, traceID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "pre-consume phase of immediate consume")
	}

	finalQuota := req.AddUsedQuota
	postReq := *req
	postPhase := ConsumePhasePost
	postReq.Phase = &postPhase
	postReq.FinalUsedQuota = &finalQuota

	transaction, updatedToken, err = processPostConsume(ctx, c, token, userID, &postReq, transaction)
	if err != nil {
		return nil, nil, errors.Wrap(err, "post-consume phase of immediate consume")
	}

	return transaction, updatedToken, nil
}

// processZeroQuotaImmediateConsume records a billing log and confirmed transaction
// for a free tool invocation. Free MCP tool calls still need a unified audit trail,
// so we persist a zero-quota log row and a finalized transaction without touching
// token or user balances.
func processZeroQuotaImmediateConsume(ctx context.Context, token *model.Token, userID int, req *consumeTokenRequest, requestID string, traceID string) (*model.TokenTransaction, *model.Token, error) {
	var transactionID string
	if req.TransactionID != nil {
		transactionID = strings.TrimSpace(*req.TransactionID)
	}
	if transactionID == "" {
		generated, err := generateTransactionID(ctx, token.Id)
		if err != nil {
			return nil, nil, errors.Wrap(err, "generate transaction id for zero-quota consume")
		}
		transactionID = generated
		req.TransactionID = &transactionID
	}

	logEntry := &model.Log{
		UserId:    userID,
		UserUUID:  token.UserUUID,
		ModelName: req.AddReason,
		TokenName: token.Name,
		TokenUUID: &token.UUID,
		Quota:     0,
		Content:   buildPostConsumeLogContent(req.AddReason, 0, 0, transactionID),
		RequestId: requestID,
		TraceId:   traceID,
	}
	if req.ElapsedTimeMs != nil && *req.ElapsedTimeMs > 0 {
		logEntry.ElapsedTime = *req.ElapsedTimeMs
	}
	model.RecordToolLog(ctx, logEntry)

	confirmedAt := helper.GetTimestamp()
	zeroQuota := int64(0)
	transaction := &model.TokenTransaction{
		TransactionID: transactionID,
		TokenId:       token.Id,
		TokenUUID:     &token.UUID,
		UserId:        userID,
		UserUUID:      token.UserUUID,
		Status:        model.TokenTransactionStatusConfirmed,
		PreQuota:      0,
		FinalQuota:    &zeroQuota,
		Reason:        req.AddReason,
		RequestId:     requestID,
		TraceId:       traceID,
		ConfirmedAt:   &confirmedAt,
		AutoConfirmed: false,
	}
	if logEntry.Id > 0 {
		logID := logEntry.Id
		transaction.LogId = &logID
	}
	if logEntry.UUID != "" {
		transaction.LogUUID = &logEntry.UUID
	}
	if req.ElapsedTimeMs != nil && *req.ElapsedTimeMs > 0 {
		elapsed := *req.ElapsedTimeMs
		transaction.ElapsedTimeMs = &elapsed
	}

	if err := model.CreateTokenTransaction(ctx, transaction); err != nil {
		return nil, nil, errors.Wrap(err, "create zero-quota token transaction")
	}

	updatedToken, err := model.GetTokenByIds(token.Id, userID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get token after zero-quota consume")
	}

	return transaction, updatedToken, nil
}

// autoConfirmExpiredTokenTransactions finalizes any pending transactions that have exceeded their timeout.
func autoConfirmExpiredTokenTransactions(ctx context.Context, c *gin.Context, tokenID int) error {
	now := helper.GetTimestamp()
	transactions, err := model.AutoConfirmExpiredTokenTransactions(ctx, tokenID, now)
	if err != nil {
		return errors.Wrap(err, "auto confirm expired token transactions")
	}

	if len(transactions) == 0 {
		return nil
	}

	logger := gmw.GetLogger(c)
	for _, txn := range transactions {
		if txn.LogId == nil {
			continue
		}
		updates := map[string]any{
			"quota":   clampQuotaToInt(txn.PreQuota),
			"content": buildAutoConfirmLogContent(txn),
		}
		if txn.ElapsedTimeMs != nil {
			updates["elapsed_time"] = *txn.ElapsedTimeMs
		}
		if err = model.UpdateConsumeLogByID(ctx, *txn.LogId, updates); err != nil {
			logger.Error("failed to update consume log after auto confirmation",
				zap.Error(err),
				zap.String("transaction_id", txn.TransactionID),
			)
		}
	}

	return nil
}

// buildTransactionResponse constructs the JSON payload describing the transaction state.
func buildTransactionResponse(txn *model.TokenTransaction) gin.H {
	if txn == nil {
		return nil
	}

	response := gin.H{
		"uuid":           txn.UUID,
		"transaction_id": txn.TransactionID,
		"token_uuid":     txn.TokenUUID,
		"user_uuid":      txn.UserUUID,
		"log_uuid":       txn.LogUUID,
		"status_code":    txn.Status,
		"status":         model.TokenTransactionStatusString(txn.Status),
		"pre_quota":      txn.PreQuota,
		"auto_confirmed": txn.AutoConfirmed,
		"expires_at":     txn.ExpiresAt,
		"reason":         txn.Reason,
		"request_id":     txn.RequestId,
		"trace_id":       txn.TraceId,
	}

	if txn.FinalQuota != nil {
		response["final_quota"] = *txn.FinalQuota
	} else {
		response["final_quota"] = nil
	}
	if txn.ConfirmedAt != nil {
		response["confirmed_at"] = *txn.ConfirmedAt
	}
	if txn.CanceledAt != nil {
		response["canceled_at"] = *txn.CanceledAt
	}
	if txn.ElapsedTimeMs != nil {
		response["elapsed_time_ms"] = *txn.ElapsedTimeMs
	}

	return response
}

// buildPreConsumeLogContent formats the log message for the pre-consume phase.
func buildPreConsumeLogContent(reason string, quota int64, transactionID string, timeoutSeconds int64) string {
	return fmt.Sprintf("External (%s) pre-consumed %s (transaction %s, timeout %ds)", reason, common.LogQuota(quota), transactionID, timeoutSeconds)
}

// buildPostConsumeLogContent formats the log message for the post-consume phase.
func buildPostConsumeLogContent(reason string, preQuota int64, finalQuota int64, transactionID string) string {
	return fmt.Sprintf("External (%s) finalized %s (pre %s, transaction %s)", reason, common.LogQuota(finalQuota), common.LogQuota(preQuota), transactionID)
}

// buildCancelConsumeLogContent formats the log message for cancellation operations.
func buildCancelConsumeLogContent(reason string, quota int64, transactionID string) string {
	return fmt.Sprintf("External (%s) canceled pre-consume hold %s (transaction %s)", reason, common.LogQuota(quota), transactionID)
}

// buildAutoConfirmLogContent formats the log message for automatic confirmations.
func buildAutoConfirmLogContent(txn *model.TokenTransaction) string {
	return fmt.Sprintf("External (%s) auto-confirmed %s (transaction %s)", txn.Reason, common.LogQuota(txn.PreQuota), txn.TransactionID)
}

// generateTransactionID creates a unique transaction identifier scoped to the
// provided token. It guarantees standard formatting and avoids collisions with
// existing transactions for the same token.
func generateTransactionID(ctx context.Context, tokenID int) (string, error) {
	for range 5 {
		candidate := random.GetUUID()
		_, err := model.GetTokenTransactionByTokenAndID(ctx, tokenID, candidate)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return candidate, nil
			}
			return "", errors.Wrap(err, "lookup token transaction candidate")
		}
	}

	return "", errors.Errorf("failed to allocate unique transaction id for token %d", tokenID)
}

// normalizeTimeoutSeconds clamps client-provided timeouts to configured limits.
func normalizeTimeoutSeconds(requested *int64) int64 {
	defaultTimeout := int64(config.ExternalBillingDefaultTimeoutSec)
	if defaultTimeout <= 0 {
		defaultTimeout = 600
	}

	timeout := defaultTimeout
	if requested != nil {
		if *requested > 0 {
			timeout = *requested
		}
	}

	maxTimeout := int64(config.ExternalBillingMaxTimeoutSec)
	if maxTimeout > 0 && timeout > maxTimeout {
		timeout = maxTimeout
	}

	return timeout
}

// quotaToInt64 converts an unsigned quota value to int64 with overflow protection.
func quotaToInt64(value uint64, fieldName string) (int64, error) {
	if value > uint64(math.MaxInt64) {
		return 0, errors.Errorf("%s exceeds supported quota range", fieldName)
	}
	return int64(value), nil
}

// clampQuotaToInt fits an int64 quota into the int range expected by logs.
func clampQuotaToInt(quota int64) int {
	if quota > math.MaxInt32 {
		return math.MaxInt32
	}
	if quota < math.MinInt32 {
		return math.MinInt32
	}
	return int(quota)
}

func validateTokenForConsumption(token *model.Token) error {
	// Check if token is enabled
	if token.Status != model.TokenStatusEnabled {
		return errors.Errorf("API Key is not enabled")
	}

	// Check if token is expired
	if token.ExpiredTime != -1 && token.ExpiredTime <= helper.GetTimestamp() {
		return errors.Errorf("The token has expired and cannot be used. Please modify the expiration time of the token, or set it to never expire")
	}

	// Check if token is exhausted
	if !token.UnlimitedQuota && token.RemainQuota <= 0 {
		return errors.Errorf("The available quota of the token has been used up. Please add more quota or set it to unlimited")
	}

	return nil
}

// GetTokenBalance retrieves the current quota status for the token authenticated in the request.
// It returns remaining quota, used quota, and whether the quota is unlimited.
func GetTokenBalance(c *gin.Context) {
	tokenId := c.GetInt(ctxkey.TokenId)
	token, err := model.GetTokenById(tokenId)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"remain_quota":    token.RemainQuota,
			"used_quota":      token.UsedQuota,
			"unlimited_quota": token.UnlimitedQuota,
		},
	})
}

// GetTokenTransactions returns a paginated history of external billing transactions
// associated with the token currently authenticated.
func GetTokenTransactions(c *gin.Context) {
	tokenId := c.GetInt(ctxkey.TokenId)
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = 10
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	startIdx := p * size
	if startIdx >= config.TokenTransactionsMaxHistory {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    []any{},
			"total":   config.TokenTransactionsMaxHistory,
		})
		return
	}

	if startIdx+size > config.TokenTransactionsMaxHistory {
		size = config.TokenTransactionsMaxHistory - startIdx
	}

	ctx := gmw.Ctx(c)
	txns, err := model.GetTokenTransactionsByTokenID(ctx, tokenId, startIdx, size)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	totalCount, err := model.GetTokenTransactionCountByTokenID(ctx, tokenId)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if totalCount > int64(config.TokenTransactionsMaxHistory) {
		totalCount = int64(config.TokenTransactionsMaxHistory)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    txns,
		"total":   totalCount,
	})
}
