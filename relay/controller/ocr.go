package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// RelayOCRHelper handles POST /v1/layout_parsing and /api/paas/v4/layout_parsing
// requests. Before dispatch it resolves the model's single pricing contract
// (token, page or per-call) and reserves a conservative allowance derived from
// the validated document selection; after the provider accepts the work it
// settles exactly once from the provider's typed receipt, keeping the allowance
// as a labelled estimate when the receipt is missing or unusable.
// Parameters: c is the request context. Returns: an API error, or nil on success.
func RelayOCRHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)

	if err := logClientRequestPayload(c, "ocr"); err != nil {
		return openai.ErrorWrapper(err, "invalid_ocr_request", http.StatusBadRequest)
	}

	ocrRequest, err := getAndValidateOCRRequest(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_ocr_request", http.StatusBadRequest)
	}

	meta.IsStream = false
	meta.OriginModelName = ocrRequest.Model
	meta.ActualModelName = metalib.GetMappedModelName(ocrRequest.Model, meta.ModelMapping)
	ocrRequest.Model = meta.ActualModelName
	metalib.Set2Context(c, meta)

	plan, bizErr := prepareOCRBillingPlan(c, meta, ocrRequest)
	if bizErr != nil {
		return bizErr
	}
	lg.Debug("prepared OCR billing plan",
		zap.String("billing_unit", string(plan.unit)),
		zap.Int("allowance_pages", plan.allowance.pages),
		zap.String("allowance_source", plan.allowance.source),
		zap.Int64("quote", plan.quote))

	meta.PromptTokens = 0

	preConsumedQuota, bizErr := preConsumeOCRQuota(c, plan.quote, meta)
	if bizErr != nil {
		return bizErr
	}
	markPreConsumed(c, preConsumedQuota)
	defer billingAuditSafetyNet(c)

	provisionalLogId := recordProvisionalLog(c, meta, ocrRequest.Model, preConsumedQuota)
	c.Set(ctxkey.ProvisionalLogId, provisionalLogId)

	adaptorImpl := relay.GetAdaptor(meta.APIType)
	if adaptorImpl == nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "invalid_api_type")
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptorImpl.Init(meta)
	ocrAdaptor, ok := adaptorImpl.(adaptor.OCRAdaptor)
	if !ok {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "ocr_not_supported")
		return openai.ErrorWrapper(errors.Errorf("OCR requests are not supported by adaptor %s", adaptorImpl.GetChannelName()), "ocr_not_supported", http.StatusBadRequest)
	}

	requestBody, err := prepareOCRRequestBody(c, meta, adaptorImpl, ocrRequest)
	if err != nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "convert_request_failed")
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}
	requestBodyBytes, err := io.ReadAll(requestBody)
	if err != nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "read_request_failed")
		return openai.ErrorWrapper(errors.Wrap(err, "read converted OCR request"), "read_request_failed", http.StatusInternalServerError)
	}

	resp, err := adaptorImpl.DoRequest(c, meta, bytes.NewReader(requestBodyBytes))
	if err != nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "do_request_failed")
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	upstreamCapture := wrapUpstreamResponse(resp)

	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)
	if requestId != "" {
		if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, plan.quote); err != nil {
			lg.Warn("record provisional user request cost failed", zap.Error(err), zap.String("request_id", requestId))
		}
	}

	if isErrorHappened(meta, resp) {
		scheduleConservativeRefund(c, preConsumedQuota, meta.TokenId, "upstream_http_error")
		if requestId != "" {
			if err := recordZeroCostAfterFailure(c, quotaId, requestId); err != nil {
				lg.Warn("update user request cost to zero failed", zap.Error(err))
			}
		}
		return RelayErrorHandlerWithContext(c, resp)
	}

	// From here the provider accepted the work: the receipt is settled exactly
	// once, including when it is missing or the client can no longer be reached.
	c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
	receipt, respErr := ocrAdaptor.DoOCRResponse(c, resp, meta)
	if upstreamCapture != nil {
		logUpstreamResponseFromCapture(lg, resp, upstreamCapture, "ocr")
	} else {
		logUpstreamResponseFromBytes(lg, resp, nil, "ocr")
	}
	settlement := plan.settle(receipt)
	if settlement.reconcileErr != nil {
		lg.Error("OCR receipt cannot be priced; conservative allowance retained, manual reconciliation required",
			zap.Error(settlement.reconcileErr),
			zap.String("billing_unit", string(plan.unit)),
			zap.Int64("retained_quota", settlement.quota))
	} else if settlement.estimateReason != "" {
		lg.Warn("OCR receipt is not verifiable; settling the conservative allowance as an estimate",
			zap.String("estimate_reason", settlement.estimateReason),
			zap.String("billing_unit", string(plan.unit)),
			zap.Int64("estimated_quota", settlement.quota))
	}
	// An error after accepted work must never be replayed on another channel.
	markResponseSettlement(c, &relaymodel.Usage{BillingEstimateReason: settlement.estimateReason}, respErr)
	recordOCRUsageMetrics(c, meta, settlement, respErr == nil)

	markBillingReconciled(c)
	runPostBillingWithTimeout(detachForBilling(c), "postBillingOCR", lg, postBillingTimeoutInfo{
		userID:              meta.UserId,
		channelID:           meta.ChannelId,
		model:               ocrRequest.Model,
		requestID:           requestId,
		startTime:           meta.StartTime,
		estimatedQuota:      func() float64 { return float64(settlement.quota) },
		guardTimeoutLog:     func() bool { return true },
		logMessage:          "CRITICAL BILLING TIMEOUT",
		includeElapsedField: true,
	}, func(ctx context.Context) {
		quota := postConsumeOCRQuota(ctx, meta, ocrRequest.Model, preConsumedQuota, plan, settlement)
		if requestId != "" {
			if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
				lg.Error("update user request cost failed", zap.Error(err), zap.String("request_id", requestId))
			}
		}
	})

	return respErr
}

// recordOCRUsageMetrics records relay, user and model metrics for one settled
// OCR request using the validated receipt counters.
// Parameters: c is the request context, meta is the relay metadata, settlement
// carries the measured counters, and delivered reports whether the response
// reached the client. It returns no value.
func recordOCRUsageMetrics(c *gin.Context, meta *metalib.Meta, settlement ocrSettlement, delivered bool) {
	userIdStr := strconv.Itoa(meta.UserId)
	username := c.GetString(ctxkey.Username)
	if username == "" {
		username = "unknown"
	}
	group := meta.Group
	if group == "" {
		group = "default"
	}
	apiFormat := c.GetString(ctxkey.APIFormat)
	if apiFormat == "" {
		apiFormat = "unknown"
	}
	channelName := channeltype.IdToName(meta.ChannelType)
	metrics.Recorder().RecordRelayRequest(meta.StartTime, meta.ChannelId, channelName, meta.ActualModelName,
		userIdStr, group, strconv.Itoa(meta.TokenId), apiFormat, relaymode.String(meta.Mode), delivered,
		settlement.promptTokens, settlement.completionTokens, 0)
	metrics.Recorder().RecordUserMetrics(userIdStr, username, group, 0,
		settlement.promptTokens, settlement.completionTokens, float64(getUserQuotaFromContext(c)))
	metrics.Recorder().RecordModelUsage(meta.ActualModelName, channelName, time.Since(meta.StartTime))
}

// getAndValidateOCRRequest decodes the OCR request body of c and checks that
// the required model and file fields are present.
// Parameters: c is the request context. Returns: the request or a wrapped validation error.
func getAndValidateOCRRequest(c *gin.Context) (*relaymodel.OCRRequest, error) {
	rawBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get request body")
	}
	_ = rawBody

	ocrRequest := &relaymodel.OCRRequest{}
	if err := common.UnmarshalBodyReusable(c, ocrRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal OCR request")
	}

	if ocrRequest.Model == "" {
		return nil, errors.New("field model is required")
	}
	if ocrRequest.File == "" {
		return nil, errors.New("field file is required")
	}

	return ocrRequest, nil
}

// prepareOCRRequestBody converts request through adaptorImpl's native OCR
// conversion, stores the converted payload on c, and returns its JSON body.
// Parameters: c is the request context, meta is unused relay metadata, adaptorImpl
// is the selected adaptor, and request is the mapped OCR request.
// Returns: the serialized body, or an error when conversion is unsupported or fails.
func prepareOCRRequestBody(c *gin.Context, meta *metalib.Meta, adaptorImpl adaptor.Adaptor, request *relaymodel.OCRRequest) (io.Reader, error) {
	if request == nil {
		return nil, errors.New("OCR request is nil")
	}

	if ocrAdaptor, ok := adaptorImpl.(adaptor.OCRAdaptor); ok {
		converted, err := ocrAdaptor.ConvertOCRRequest(c, request)
		if err != nil {
			return nil, errors.Wrap(err, "convert OCR request")
		}
		c.Set(ctxkey.ConvertedRequest, converted)

		payload, err := json.Marshal(converted)
		if err != nil {
			return nil, errors.Wrap(err, "marshal OCR request")
		}
		return bytes.NewBuffer(payload), nil
	}

	channelName := adaptorImpl.GetChannelName()
	if channelName == "" {
		channelName = "unknown"
	}
	return nil, errors.Errorf("OCR requests are not supported by adaptor %s", channelName)
}

// preConsumeOCRQuota reserves the plan's conservative quote against the durable
// user and token balances before dispatch. A zero quote (an explicit free tariff)
// reserves nothing.
// Parameters: c is the request context, quote is the planned allowance, and meta
// is the relay metadata. Returns: the reserved quota or an admission error.
func preConsumeOCRQuota(c *gin.Context, quote int64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	return reservePaidRequestQuota(c, meta, max(int64(0), quote), "ocr_preconsume")
}

// postConsumeOCRQuota records the single final OCR charge: it moves the ledger
// by the difference between the settled charge and the reservation, and
// reconciles the provisional consume log with the billing unit, allowance and
// receipt provenance. It runs on a detached billing context and never reads a
// live *gin.Context.
// Parameters: ctx is the detached billing context, meta is the relay metadata,
// modelName is the billed model, preConsumedQuota is the reservation, plan is
// the pre-dispatch billing plan, and settlement is the receipt-derived charge.
// Returns: the final charge submitted for settlement.
func postConsumeOCRQuota(ctx context.Context,
	meta *metalib.Meta,
	modelName string,
	preConsumedQuota int64,
	plan ocrBillingPlan,
	settlement ocrSettlement) int64 {
	quota := max(settlement.quota, 0)

	// Resolve identifiers from the detached billing snapshot (or, for a synchronous
	// caller, from the embedded gin context). NEVER read them off a live *gin.Context
	// here: this can run inside a post-billing goroutine and gin recycles c.
	billingID := billingIdentityFromContext(ctx)
	requestId := billingID.requestID
	provLogID := billingID.provisionalLogID
	traceId := billingID.traceID

	if meta.TokenId <= 0 || meta.UserId <= 0 || meta.ChannelId <= 0 {
		gmw.GetLogger(ctx).Error("meta information incomplete, cannot post consume OCR quota",
			zap.Int("meta_token_id", meta.TokenId),
			zap.Int("meta_user_id", meta.UserId),
			zap.Int("meta_channel_id", meta.ChannelId),
			zap.String("request_id", requestId),
			zap.String("trace_id", traceId),
		)
		return quota
	}

	metadata := model.LogMetadata{
		"ocr_billing_unit":     string(plan.unit),
		"ocr_allowance_pages":  plan.allowance.pages,
		"ocr_allowance_source": plan.allowance.source,
		"ocr_allowance_quota":  plan.quote,
	}
	if settlement.pages > 0 {
		metadata["ocr_receipt_pages"] = settlement.pages
	}
	if settlement.exceedsAllowance {
		metadata["ocr_receipt_exceeds_allowance"] = true
	}
	logEntry := &model.Log{
		UserId:             meta.UserId,
		ChannelId:          meta.ChannelId,
		PromptTokens:       settlement.promptTokens,
		CompletionTokens:   settlement.completionTokens,
		CachedPromptTokens: settlement.cachedTokens,
		ModelName:          modelName,
		TokenName:          meta.TokenName,
		Content: fmt.Sprintf("OCR %s billing, %d admitted page(s), group rate %.2f",
			plan.unit, plan.allowance.pages, plan.groupRatio),
		Metadata:    billingEstimateMetadata(metadata, settlement.estimateReason),
		IsStream:    false,
		ElapsedTime: helper.CalcElapsedTime(meta.StartTime),
		RequestId:   requestId,
		TraceId:     traceId,
	}
	model.SetLogExternalUUIDs(logEntry, meta.UserUUID, meta.ChannelUUID, meta.TokenUUID)
	billing.PostConsumeQuotaWithLog(ctx, meta.TokenId, quota-preConsumedQuota, quota, logEntry, provLogID)
	return quota
}
