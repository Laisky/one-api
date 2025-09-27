package controller

import (
	"context"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/constant/role"
	"github.com/songquanpeng/one-api/relay/controller/validator"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func getAndValidateTextRequest(c *gin.Context, relayMode int) (*relaymodel.GeneralOpenAIRequest, error) {
	// Check for unknown parameters first
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get request body")
	}

	// Validate for unknown parameters requests
	if err = validator.ValidateUnknownParameters(requestBody); err != nil {
		return nil, errors.Wrap(err, "unknown parameter validation failed")
	}

	textRequest := &relaymodel.GeneralOpenAIRequest{}
	err = common.UnmarshalBodyReusable(c, textRequest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal request body")
	}
	if relayMode == relaymode.Moderations && textRequest.Model == "" {
		textRequest.Model = "text-moderation-latest"
	}
	if relayMode == relaymode.Embeddings && textRequest.Model == "" {
		textRequest.Model = c.Param("model")
	}
	err = validator.ValidateTextRequest(textRequest, relayMode)
	if err != nil {
		return nil, errors.Wrap(err, "text request validation failed")
	}
	return textRequest, nil
}

// For Realtime websocket sessions, upgrade and proxy immediately.
// This keeps the rest of the text pipeline unchanged for other modes.
func maybeHandleRealtime(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	m := meta.GetByContext(c)
	if m.Mode == relaymode.Realtime && m.ChannelType == channeltype.OpenAI {
		if bizErr, _ := openai.RealtimeHandler(c, m); bizErr != nil {
			return bizErr
		}
		return nil
	}
	return nil
}

func getPromptTokens(ctx context.Context, textRequest *relaymodel.GeneralOpenAIRequest, relayMode int) int {
	switch relayMode {
	case relaymode.ChatCompletions:
		actualModel := textRequest.Model
		// video request
		if strings.HasPrefix(actualModel, "veo-") {
			return ratio.TokensPerSec * 8
		}

		// text request
		return openai.CountTokenMessages(ctx, textRequest.Messages, textRequest.Model)
	case relaymode.Completions:
		return openai.CountTokenInput(textRequest.Prompt, textRequest.Model)
	case relaymode.Moderations:
		return openai.CountTokenInput(textRequest.Input, textRequest.Model)
	case relaymode.Embeddings:
		// Use ParseInput to properly handle both string and array inputs
		inputs := textRequest.ParseInput()
		totalTokens := 0
		for _, input := range inputs {
			totalTokens += openai.CountTokenText(input, textRequest.Model)
		}
		return totalTokens
	case relaymode.Rerank:
		return openai.CountTokenInput(textRequest.Input, textRequest.Model)
	case relaymode.Edits:
		return openai.CountTokenInput(textRequest.Instruction, textRequest.Model)
	default:
		// Log error for unhandled relay modes that should have billing
		gmw.GetLogger(ctx).Error("getPromptTokens: unhandled relay mode without billing logic",
			zap.Int("relayMode", relayMode),
			zap.String("model", textRequest.Model))
	}

	return 0
}

func getPreConsumedQuota(textRequest *relaymodel.GeneralOpenAIRequest, promptTokens int, ratio float64) int64 {
	preConsumedTokens := config.PreConsumedQuota + int64(promptTokens)
	// Prefer max_completion_tokens; fall back to deprecated max_tokens
	if textRequest.MaxCompletionTokens != nil && *textRequest.MaxCompletionTokens > 0 {
		preConsumedTokens += int64(*textRequest.MaxCompletionTokens)
	} else if textRequest.MaxTokens != 0 {
		preConsumedTokens += int64(textRequest.MaxTokens)
	}

	baseQuota := int64(float64(preConsumedTokens) * ratio)
	return baseQuota
}

func preConsumeQuota(c *gin.Context, textRequest *relaymodel.GeneralOpenAIRequest, promptTokens int, ratio float64, meta *meta.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	lg := gmw.GetLogger(c)
	preConsumedQuota := getPreConsumedQuota(textRequest, promptTokens, ratio)

	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	userQuota, err := model.CacheGetUserQuota(gmw.Ctx(c), meta.UserId)
	if err != nil {
		return preConsumedQuota, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota-preConsumedQuota < 0 {
		return preConsumedQuota, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}
	err = model.CacheDecreaseUserQuota(meta.UserId, preConsumedQuota)
	if err != nil {
		return preConsumedQuota, openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota > 100*preConsumedQuota &&
		(tokenQuotaUnlimited || tokenQuota > 100*preConsumedQuota) {
		// in this case, we do not pre-consume quota
		// because the user and token have enough quota
		preConsumedQuota = 0
		lg.Info("user has enough quota, trusted and no need to pre-consume", zap.Int("user_id", meta.UserId), zap.Int64("user_quota", userQuota))
	}
	if preConsumedQuota > 0 {
		err := model.PreConsumeTokenQuota(meta.TokenId, preConsumedQuota)
		if err != nil {
			return preConsumedQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
	}
	return preConsumedQuota, nil
}

func postConsumeQuota(ctx context.Context,
	usage *relaymodel.Usage,
	meta *meta.Meta,
	textRequest *relaymodel.GeneralOpenAIRequest,
	ratio float64,
	preConsumedQuota int64,
	incrementallyCharged int64,
	modelRatio float64,
	groupRatio float64,
	systemPromptReset bool,
	channelCompletionRatio map[string]float64) (quota int64) {
	if usage == nil {
		gmw.GetLogger(ctx).Error("usage is nil, which is unexpected")
		return
	}

	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              textRequest.Model,
		ModelRatio:             modelRatio,
		GroupRatio:             groupRatio,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
	})

	quota = computeResult.TotalQuota
	totalTokens := computeResult.PromptTokens + computeResult.CompletionTokens
	if totalTokens == 0 {
		quota = 0
	}

	quotaDelta := quota - preConsumedQuota - incrementallyCharged
	// Derive RequestId/TraceId from std context if possible (gin ctx embedded by gmw.BackgroundCtx)
	var requestId string
	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		requestId = ginCtx.GetString(ctxkey.RequestId)
	}
	traceId := tracing.GetTraceIDFromContext(ctx)
	billing.PostConsumeQuotaDetailed(billing.QuotaConsumeDetail{
		Ctx:                    ctx,
		TokenId:                meta.TokenId,
		QuotaDelta:             quotaDelta,
		TotalQuota:             quota,
		UserId:                 meta.UserId,
		ChannelId:              meta.ChannelId,
		PromptTokens:           computeResult.PromptTokens,
		CompletionTokens:       computeResult.CompletionTokens,
		ModelRatio:             computeResult.UsedModelRatio,
		GroupRatio:             groupRatio,
		ModelName:              textRequest.Model,
		TokenName:              meta.TokenName,
		IsStream:               meta.IsStream,
		StartTime:              meta.StartTime,
		SystemPromptReset:      systemPromptReset,
		CompletionRatio:        computeResult.UsedCompletionRatio,
		ToolsCost:              usage.ToolsCost,
		CachedPromptTokens:     computeResult.CachedPromptTokens,
		CachedCompletionTokens: 0,
		RequestId:              requestId,
		TraceId:                traceId,
	})

	return quota
}

// postConsumeQuotaWithTraceID is deprecated; callers should pass IDs via QuotaConsumeDetail
func postConsumeQuotaWithTraceID(ctx context.Context, traceId string,
	usage *relaymodel.Usage,
	meta *meta.Meta,
	textRequest *relaymodel.GeneralOpenAIRequest,
	ratio float64,
	preConsumedQuota int64,
	modelRatio float64,
	groupRatio float64,
	systemPromptReset bool,
	channelCompletionRatio map[string]float64) (quota int64) {
	if usage == nil {
		gmw.GetLogger(ctx).Error("usage is nil, which is unexpected")
		return
	}

	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              textRequest.Model,
		ModelRatio:             modelRatio,
		GroupRatio:             groupRatio,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
	})

	quota = computeResult.TotalQuota
	totalTokens := computeResult.PromptTokens + computeResult.CompletionTokens
	if totalTokens == 0 {
		quota = 0
	}

	quotaDelta := quota - preConsumedQuota
	var requestId string
	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		requestId = ginCtx.GetString(ctxkey.RequestId)
	}
	billing.PostConsumeQuotaDetailed(billing.QuotaConsumeDetail{
		Ctx:                    ctx,
		TokenId:                meta.TokenId,
		QuotaDelta:             quotaDelta,
		TotalQuota:             quota,
		UserId:                 meta.UserId,
		ChannelId:              meta.ChannelId,
		PromptTokens:           computeResult.PromptTokens,
		CompletionTokens:       computeResult.CompletionTokens,
		ModelRatio:             computeResult.UsedModelRatio,
		GroupRatio:             groupRatio,
		ModelName:              textRequest.Model,
		TokenName:              meta.TokenName,
		IsStream:               meta.IsStream,
		StartTime:              meta.StartTime,
		SystemPromptReset:      systemPromptReset,
		CompletionRatio:        computeResult.UsedCompletionRatio,
		ToolsCost:              usage.ToolsCost,
		CachedPromptTokens:     computeResult.CachedPromptTokens,
		CachedCompletionTokens: 0,
		RequestId:              requestId,
		TraceId:                traceId,
	})

	return quota
}

func isErrorHappened(meta *meta.Meta, resp *http.Response) bool {
	if resp == nil {
		if meta.ChannelType == channeltype.AwsClaude {
			return false
		}
		return true
	}
	if resp.StatusCode != http.StatusOK &&
		// replicate return 201 to create a task
		resp.StatusCode != http.StatusCreated {
		return true
	}
	if meta.ChannelType == channeltype.DeepL {
		// skip stream check for deepl
		return false
	}

	if meta.IsStream && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") &&
		// Even if stream mode is enabled, replicate will first return a task info in JSON format,
		// requiring the client to request the stream endpoint in the task info
		meta.ChannelType != channeltype.Replicate {
		return true
	}
	return false
}

func setSystemPrompt(ctx context.Context, request *relaymodel.GeneralOpenAIRequest, prompt string) (reset bool) {
	if prompt == "" {
		return false
	}
	if len(request.Messages) == 0 {
		return false
	}
	lg := gmw.GetLogger(ctx)
	if request.Messages[0].Role == role.System {
		request.Messages[0].Content = prompt
		lg.Info("rewrite system prompt", zap.String("prompt", prompt))
		return true
	}
	request.Messages = append([]relaymodel.Message{{
		Role:    role.System,
		Content: prompt,
	}}, request.Messages...)
	lg.Info("add system prompt", zap.String("prompt", prompt))
	return true
}
