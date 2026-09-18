package controller

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/typesafe"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// RelaySystemOne handles native evaluation under the standard authenticated relay
// middleware. It makes one upstream attempt: there is no invisible paid replay.
// Native results are buffered until the durable balance adjustment succeeds.
func RelaySystemOne(c *gin.Context) {
	c.Set(ctxkey.APIFormat, "systemone")
	result, usage, apiErr := runSystemOne(c)
	m := metalib.GetByContext(c)
	if usage == nil {
		usage = &relaymodel.Usage{}
	}
	metrics.Recorder().RecordRelayRequest(m.StartTime, m.ChannelId, "typesafe", m.ActualModelName,
		strconv.Itoa(m.UserId), m.Group, strconv.Itoa(m.TokenId), "systemone", "systemone",
		apiErr == nil, usage.PromptTokens, usage.CompletionTokens, 0)
	metrics.Recorder().RecordModelUsage(m.ActualModelName, "typesafe", time.Since(m.StartTime))
	if usage.BillingEstimateReason != "" {
		c.Header("X-OneAPI-Billing-Estimated", "true")
	}
	if c.Writer.Written() {
		return
	}
	if result != nil {
		result.Write(c)
	} else if apiErr != nil {
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.Error})
	}
}

// runSystemOne validates and maps the native request, reserves its input allowance,
// performs inference, and settles exactly one attempt using input-only pricing.
func runSystemOne(c *gin.Context) (result *typesafe.Response, usage *relaymodel.Usage, apiErr *relaymodel.ErrorWithStatusCode) {
	m := metalib.GetByContext(c)
	if m.ChannelType != channeltype.TypeSafe || m.Mode != relaymode.SystemOne {
		return nil, nil, openai.ErrorWrapper(errors.New("System One requires a TypeSafe channel"), "unsupported_systemone_channel", http.StatusBadRequest)
	}
	provider, ok := relay.GetAdaptor(m.APIType).(*typesafe.Adaptor)
	if !ok {
		return nil, nil, openai.ErrorWrapper(errors.New("TypeSafe adaptor unavailable"), "invalid_api_type", http.StatusBadRequest)
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "invalid_systemone_request", http.StatusBadRequest)
	}
	request, err := typesafe.DecodeRequest(body)
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "invalid_systemone_request", http.StatusBadRequest)
	}
	m.OriginModelName = request.Model
	m.ActualModelName = metalib.GetMappedModelName(request.Model, m.ModelMapping)
	request.Model = m.ActualModelName
	m.IsStream = false
	metalib.Set2Context(c, m)
	c.Set(ctxkey.ConvertedRequest, request)
	provider.Init(m)
	if _, err := provider.GetRequestURL(m); err != nil {
		return nil, nil, openai.ErrorWrapper(err, "invalid_typesafe_url", http.StatusBadRequest)
	}
	configs := getChannelModelConfigs(c)
	inputOverrides, _ := getChannelRatios(c)
	config, found := pricing.ResolveModelConfig(request.Model, configs, provider, m.StartTime)
	if !found || len(config.Tiers) > 0 || config.PerCall != nil {
		return nil, nil, openai.ErrorWrapper(errors.New("TypeSafe requires known flat input-token pricing"), "unsupported_typesafe_pricing", http.StatusBadRequest)
	}
	inputRatio := pricing.ResolveModelRatioAt(request.Model, configs, inputOverrides, provider, m.StartTime)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	reserved, err := typesafe.InputQuota(typesafe.AdmissionInputTokens, inputRatio, groupRatio)
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "invalid_typesafe_pricing", http.StatusBadRequest)
	}
	body, err = json.Marshal(request)
	if err != nil {
		return nil, nil, openai.ErrorWrapper(err, "invalid_systemone_request", http.StatusBadRequest)
	}
	if reserved > 0 {
		if err := model.PreConsumeTokenQuota(gmw.Ctx(c), m.TokenId, reserved); err != nil {
			return nil, nil, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
	}
	var provisionalID int
	usage = typesafe.EstimatedUsage("typesafe_unresolved_upstream_attempt")
	// Install settlement before dispatch, including panic/cancellation paths. Do
	// not use the generic refund safety net: an ambiguous attempt may be paid.
	defer func() {
		if !c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded) {
			usage = &relaymodel.Usage{}
		}
		if usage == nil {
			usage = typesafe.EstimatedUsage("typesafe_unresolved_upstream_attempt")
		}
		if err := settleSystemOne(c, m, usage, reserved, inputRatio, groupRatio, provisionalID); err != nil {
			result = nil
			apiErr = openai.ErrorWrapper(err, "typesafe_settlement_failed", http.StatusInternalServerError)
		}
	}()
	markPreConsumed(c, reserved)
	if reserved > 0 {
		syncUserQuotaCacheAfterPreConsume(gmw.Ctx(c), m.UserId, reserved, "typesafe_preconsume")
	}
	provisionalID = recordProvisionalLog(c, m, request.Model, reserved)
	c.Set(ctxkey.ProvisionalLogId, provisionalID)
	response, err := provider.DoRequest(c, m, bytes.NewReader(body))
	if err != nil {
		return nil, usage, openai.ErrorWrapper(err, "typesafe_request_failed", http.StatusBadGateway)
	}
	result, usage, apiErr = provider.ReadResponse(c, response, m)
	return result, usage, apiErr
}

// settleSystemOne applies only final-minus-reserved to durable balances. A known
// debit failure leaves the provisional record unresolved and prevents success
// output. Log/cache failures after a successful debit never trigger another debit.
func settleSystemOne(c *gin.Context, m *metalib.Meta, usage *relaymodel.Usage, reserved int64,
	inputRatio, groupRatio float64, provisionalID int) error {
	lg := gmw.GetLogger(c)
	total, err := typesafe.InputQuota(usage.PromptTokens, inputRatio, groupRatio)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(relayctx.Detach(c), 30*time.Second)
	defer cancel()
	if err := model.SettleConsumedTokenQuota(ctx, m.TokenId, m.UserId, total-reserved); err != nil {
		return errors.Wrap(err, "settle TypeSafe input charge; retained reservation requires reconciliation")
	}
	markBillingReconciled(c)
	if err := model.CacheUpdateUserQuota(ctx, m.UserId); err != nil {
		lg.Warn("refresh TypeSafe user quota cache failed after settlement", zap.Error(err))
	}
	entry := &model.Log{
		UserId: m.UserId, ChannelId: m.ChannelId, ModelName: m.ActualModelName,
		OriginModelName: m.OriginModelName, ElapsedTime: helper.CalcElapsedTime(m.StartTime),
		TokenName: m.TokenName, RequestId: c.GetString(ctxkey.RequestId),
		PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		Quota: int(total), Content: "TypeSafe System One input-token charge; output tokens are free",
	}
	model.SetLogExternalUUIDs(entry, m.UserUUID, m.ChannelUUID, m.TokenUUID)
	if usage.BillingEstimateReason != "" {
		entry.Metadata = billingEstimateMetadata(nil, usage.BillingEstimateReason)
		entry.Content = "Estimated TypeSafe input charge: upstream usage could not be verified"
	}
	if provisionalID > 0 {
		err := model.ReconcileConsumeLogDetailed(ctx, provisionalID, model.ConsumeLogReconcileDetail{
			FinalQuota: total, Content: entry.Content, PromptTokens: entry.PromptTokens,
			CompletionTokens: entry.CompletionTokens, ElapsedTime: entry.ElapsedTime, Metadata: entry.Metadata,
		})
		if err != nil {
			lg.Error("reconcile TypeSafe consume log failed after settlement", zap.Error(err))
			model.RecordConsumeLog(ctx, entry)
		}
	} else {
		model.RecordConsumeLog(ctx, entry)
	}
	if total > 0 {
		model.UpdateUserUsedQuotaAndRequestCountWithContext(ctx, m.UserId, total)
		model.UpdateChannelUsedQuotaWithContext(ctx, m.ChannelId, total)
	}
	if entry.RequestId != "" {
		if err := model.UpdateUserRequestCostQuotaByRequestID(m.UserId, entry.RequestId, total); err != nil {
			lg.Error("record TypeSafe request cost failed after settlement", zap.Error(err))
		}
	}
	return nil
}
