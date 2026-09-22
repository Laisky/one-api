package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

type commonAudioRequest struct {
	File *multipart.FileHeader `form:"file" binding:"required"`
}

// extractAudioModelFromMultipart reads the cached request body and binds the `model` form field.
// On any error or missing field, it returns an empty string and leaves the original body reusable.
func extractAudioModelFromMultipart(c *gin.Context) string {
	body, err := common.GetRequestBody(c)
	if err != nil || len(body) == 0 {
		return ""
	}
	// Restore body for binding
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var req struct {
		Model string `form:"model"`
	}
	if err := c.ShouldBind(&req); err != nil {
		// Reset body and ignore error
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		return ""
	}
	// Reset body for downstream usage
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return req.Model
}

func countAudioTokens(c *gin.Context, tokensPerSecond float64) (float64, error) {
	body, err := common.GetRequestBody(c)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	reqBody := new(commonAudioRequest)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err = c.ShouldBind(reqBody); err != nil {
		return 0, errors.WithStack(err)
	}

	reqFp, err := reqBody.File.Open()
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer reqFp.Close()

	return helper.GetAudioTokens(gmw.Ctx(c),
		reqFp,
		tokensPerSecond)
}

// RelayAudioHelper normalizes one standard audio request, meters its actual input
// unit and settles accepted work even if writing the result to the caller fails.
func RelayAudioHelper(c *gin.Context, relayMode int) *relaymodel.ErrorWithStatusCode {
	ctx := gmw.Ctx(c)
	meta := meta.GetByContext(c)
	audioModel := "whisper-1"

	tokenId := c.GetInt(ctxkey.TokenId)
	channelType := c.GetInt(ctxkey.Channel)
	channelId := c.GetInt(ctxkey.ChannelId)
	userId := c.GetInt(ctxkey.Id)
	// group := c.GetString(ctxkey.Group)
	tokenName := c.GetString(ctxkey.TokenName)

	var ttsRequest openai.TextToSpeechRequest
	if relayMode == relaymode.AudioSpeech {
		// Read JSON
		err := common.UnmarshalBodyReusable(c, &ttsRequest)
		// Check if JSON is valid
		if err != nil {
			return openai.ErrorWrapper(err, "invalid_json", http.StatusBadRequest)
		}
		audioModel = ttsRequest.Model
		// Check if text is too long 4096
		if utf8.RuneCountInString(ttsRequest.Input) > 4096 {
			return openai.ErrorWrapper(errors.New("input is too long (over 4096 characters)"), "text_too_long", http.StatusBadRequest)
		}
	} else if relayMode == relaymode.AudioTranscription || relayMode == relaymode.AudioTranslation {
		// Extract `model` from multipart form for transcription/translation
		if m := extractAudioModelFromMultipart(c); m != "" {
			audioModel = m
		}
	}

	if strings.TrimSpace(audioModel) == "" {
		return openai.ErrorWrapper(errors.New("audio model must not be empty"), "invalid_audio_model", http.StatusBadRequest)
	}
	originalAudioModel := audioModel
	meta.OriginModelName = audioModel
	modelMapping := c.GetStringMapString(ctxkey.ModelMapping)
	if mapped := modelMapping[audioModel]; mapped != "" {
		audioModel = mapped
	}
	pricingModel := audioModel
	// Keep historical slugs usable without bypassing their administrator tariff.
	if channelType == channeltype.Mistral && audioModel == "voxtral-tts-2603" {
		audioModel = "voxtral-mini-tts-2603"
	}
	if channelType == channeltype.Mistral && audioModel == "voxtral-mini-transcribe-2602" {
		audioModel = "voxtral-mini-2602"
	}
	meta.ActualModelName = audioModel
	if (channelType == channeltype.Mistral || channelType == channeltype.Zhipu || channelType == channeltype.Zai) && relayMode == relaymode.AudioTranslation {
		return openai.ErrorWrapper(errors.New("this channel supports transcription, not a standard translation endpoint"), "unsupported_audio_translation", http.StatusBadRequest)
	}
	if channelType == channeltype.Groq && audioModel == "whisper-large-v3-turbo" && relayMode == relaymode.AudioTranslation {
		return openai.ErrorWrapper(errors.New("Whisper Large V3 Turbo supports transcription only"), "unsupported_audio_translation", http.StatusBadRequest)
	}
	if relayMode == relaymode.AudioSpeech && strings.TrimSpace(ttsRequest.Input) == "" {
		return openai.ErrorWrapper(errors.New("speech input must not be empty"), "invalid_audio_input", http.StatusBadRequest)
	}
	wireBody, wireContentType, err := normalizeAudioWire(c, relayMode, channelType, audioModel, &ttsRequest)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_audio_request", http.StatusBadRequest)
	}

	// get channel-specific pricing if available
	var channelModelRatio map[string]float64
	var channelModelConfigs map[string]model.ModelConfigLocal
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			// Get from unified ModelConfigs only (after migration)
			channelModelRatio = channel.GetModelRatioFromConfigsWithContext(ctx)
			channelModelConfigs = channel.GetModelPriceConfigsWithContext(ctx)
		}
	}

	// Use three-layer pricing system
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.ResolveModelRatioAt(pricingModel, channelModelConfigs, channelModelRatio, pricingAdaptor, meta.StartTime)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	audioPricingCfg, hasAudioPricing := pricing.ResolveAudioPricing(pricingModel, channelModelConfigs, pricingAdaptor, meta.StartTime)
	tokensPerSecond := pricing.DefaultAudioPromptTokensPerSecond
	if hasAudioPricing && audioPricingCfg != nil && audioPricingCfg.PromptTokensPerSecond > 0 {
		tokensPerSecond = audioPricingCfg.PromptTokensPerSecond
	}
	seconds := float64(0)
	if relayMode == relaymode.AudioTranscription || relayMode == relaymode.AudioTranslation {
		measured, err := countAudioTokens(c, 1)
		if err != nil {
			return openai.ErrorWrapper(err, "invalid_audio_duration", http.StatusBadRequest)
		}
		seconds = measured
	} else if relayMode != relaymode.AudioSpeech {
		return openai.ErrorWrapper(errors.New("unexpected audio relay mode"), "unexpected_relay_mode", http.StatusBadRequest)
	}
	local, hasLocal := channelModelConfigs[pricingModel]
	ratioOverride := hasLocal && local.Ratio != 0 && local.Audio == nil
	quota, unit, err := quoteAudioInput(relayMode, ttsRequest.Input, seconds, tokensPerSecond, modelRatio, groupRatio, audioPricingCfg, ratioOverride)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_audio_pricing", http.StatusBadRequest)
	}
	preConsumedQuota := quota

	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	userQuota, err := model.CacheGetUserQuota(ctx, userId)
	if err != nil {
		return openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}

	// Check if user quota is enough
	if userQuota-preConsumedQuota < 0 {
		return openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}
	if preConsumedQuota < userQuota/100 &&
		(tokenQuotaUnlimited || preConsumedQuota < tokenQuota/100) {
		// in this case, we do not pre-consume quota
		// because the user has enough quota
		preConsumedQuota = 0
	}
	if preConsumedQuota > 0 {
		err := model.PreConsumeTokenQuota(ctx, tokenId, preConsumedQuota)
		if err != nil {
			return openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
		syncUserQuotaCacheAfterPreConsume(ctx, userId, preConsumedQuota, "audio_preconsume")

		// Billing audit safety net
		markPreConsumed(c, preConsumedQuota)
		defer billingAuditSafetyNet(c)

		provisionalLogId := recordProvisionalLog(c, meta, originalAudioModel, preConsumedQuota)
		c.Set(ctxkey.ProvisionalLogId, provisionalLogId)
	}
	provLogID := c.GetInt(ctxkey.ProvisionalLogId)
	lg := gmw.GetLogger(c)
	succeed := false
	defer func() {
		if succeed {
			return
		}
		if provLogID > 0 {
			if err := model.ReconcileConsumeLog(detachForBilling(c), provLogID, 0, "audio request failed, refunded", 0, 0, 0, nil); err != nil {
				lg.Warn("reconcile failed audio consume log", zap.Error(err))
			}
		}
		if err := model.UpdateUserRequestCostQuotaByRequestID(userId, c.GetString(ctxkey.RequestId), 0); err != nil {
			lg.Warn("reconcile failed audio request cost", zap.Error(err))
		}
		markBillingReconciled(c)
		if preConsumedQuota > 0 {
			// we need to roll back the pre-consumed quota under lifecycle tracking
			goAudioRollbackPreConsumed(c, tokenId, preConsumedQuota)
		}
	}()

	baseURL := channeltype.ChannelBaseURLs[channelType]
	requestURL := c.Request.URL.String()
	if c.GetString(ctxkey.BaseURL) != "" {
		baseURL = c.GetString(ctxkey.BaseURL)
	}

	fullRequestURL := openai.GetFullRequestURL(baseURL, requestURL, channelType)
	if channelType == channeltype.Azure {
		apiVersion := meta.Config.APIVersion
		switch relayMode {
		case relaymode.AudioTranscription:
			// https://learn.microsoft.com/en-us/azure/ai-services/openai/whisper-quickstart?tabs=command-line#rest-api
			fullRequestURL = fmt.Sprintf("%s/openai/deployments/%s/audio/transcriptions?api-version=%s", baseURL, audioModel, apiVersion)
		case relaymode.AudioSpeech:
			// https://learn.microsoft.com/en-us/azure/ai-services/openai/text-to-speech-quickstart?tabs=command-line#rest-api
			fullRequestURL = fmt.Sprintf("%s/openai/deployments/%s/audio/speech?api-version=%s", baseURL, audioModel, apiVersion)
		}
	}
	if channelType == channeltype.Zhipu || channelType == channeltype.Zai {
		baseURL = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/api/paas/v4")
		// Zhipu and Z.AI are the same platform under two brands and expose the same
		// OpenAI-compatible audio endpoints under /api/paas/v4. Z.AI serves only
		// transcription (GLM-ASR-2512); it publishes no text-to-speech model.
		// Sources: https://docs.bigmodel.cn/api-reference/模型-api/文本转语音
		// https://docs.bigmodel.cn/api-reference/模型-api/语音转文本
		// https://docs.z.ai/api-reference/audio/audio-transcriptions
		switch relayMode {
		case relaymode.AudioSpeech:
			fullRequestURL = fmt.Sprintf("%s/api/paas/v4/audio/speech", baseURL)
		case relaymode.AudioTranscription:
			fullRequestURL = fmt.Sprintf("%s/api/paas/v4/audio/transcriptions", baseURL)
		case relaymode.AudioTranslation:
			return openai.ErrorWrapper(
				errors.New("this provider does not offer an audio translation endpoint; GLM-ASR-2512 supports multilingual transcription via /v1/audio/transcriptions"),
				"unsupported_audio_translation", http.StatusBadRequest)
		}
	}

	// Dispatch only this attempt's normalized bytes. Keep the client body and
	// transport headers intact for another channel's mapping and audio metering.
	req, err := http.NewRequestWithContext(ctx, c.Request.Method, fullRequestURL, bytes.NewReader(wireBody))
	if err != nil {
		return openai.ErrorWrapper(err, "new_request_failed", http.StatusInternalServerError)
	}

	if (relayMode == relaymode.AudioTranscription || relayMode == relaymode.AudioSpeech) && channelType == channeltype.Azure {
		// https://learn.microsoft.com/en-us/azure/ai-services/openai/whisper-quickstart?tabs=command-line#rest-api
		apiKey := c.Request.Header.Get("Authorization")
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		req.Header.Set("api-key", apiKey)
	} else {
		req.Header.Set("Authorization", c.Request.Header.Get("Authorization"))
	}
	req.Header.Set("Content-Type", wireContentType)
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))

	// Record what the caller actually asked for. The multipart body is excluded from
	// the generic request logger, so without this an upstream parameter rejection
	// leaves no gateway-side evidence of the request that caused it.
	logAudioRequestParameters(c, relayMode, audioModel, &ttsRequest)
	// Log upstream request for billing tracking
	lg.Info("sending audio request to upstream channel",
		zap.String("url", fullRequestURL),
		zap.String("model", audioModel),
		zap.Int("relay_mode", relayMode))

	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		// Let ErrorWrapper handle the logging to avoid duplicate logging
		return openai.ErrorWrapper(errors.Wrapf(err, "upstream audio request failed for channel %d", channelId), "do_request_failed", http.StatusInternalServerError)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			lg.Warn("close upstream audio response", zap.Error(err))
		}
	}()

	// Immediately record a provisional request cost using the estimated quota, even if we skipped physical pre-consume
	// (trusted path). This ensures cancellation cases are still tracked and later reconciled.
	{
		requestId := c.GetString(ctxkey.RequestId)
		if err := model.UpdateUserRequestCostQuotaByRequestID(userId, requestId, quota); err != nil {
			lg.Warn("record provisional user request cost failed", zap.Error(err))
		}
	}

	if resp.StatusCode != http.StatusOK {
		return RelayErrorHandler(resp)
	}
	if err := normalizeAudioResponse(resp, relayMode, channelType, ttsRequest.ResponseFormat); err != nil {
		return openai.ErrorWrapper(err, "invalid_audio_response", http.StatusBadGateway)
	}

	succeed = true
	c.Set(adaptor.AudioReceiptAcceptedKey, true)
	markBillingReconciled(c)
	quotaDelta := quota - preConsumedQuota

	// Capture trace ID from gin context now; the background context will not carry gin
	var traceID string
	if tid, err := gmw.TraceID(c); err == nil {
		traceID = tid.String()
	}

	defer func() {
		bgctx, cancel := context.WithTimeout(detachForBilling(c), time.Minute)
		defer cancel()

		// Build a full log entry with IDs from gin.Context
		logContent := fmt.Sprintf("audio input unit %s, model rate %.8g, group rate %.8g", unit, modelRatio, groupRatio)
		entry := &model.Log{
			UserId:           userId,
			UserUUID:         model.StringPtrIfNotEmpty(meta.UserUUID),
			ChannelId:        channelId,
			ChannelUUID:      model.StringPtrIfNotEmpty(meta.ChannelUUID),
			PromptTokens:     0, // Input units are not model tokens; the exact tariff is in Content.
			CompletionTokens: 0,
			ModelName:        originalAudioModel,
			TokenName:        tokenName,
			TokenUUID:        model.StringPtrIfNotEmpty(meta.TokenUUID),
			Content:          logContent,
			RequestId:        c.GetString(ctxkey.RequestId),
			TraceId:          traceID,
			ElapsedTime:      helper.CalcElapsedTime(meta.StartTime), // capture request latency in ms
		}
		graceful.GoCritical(bgctx, "audioPostConsumeWithLog", func(cctx context.Context) {
			billing.PostConsumeQuotaWithLog(cctx, tokenId, quotaDelta, quota, entry, provLogID)
		})

		// Reconcile user request cost to final quota (override provisional value)
		if err := model.UpdateUserRequestCostQuotaByRequestID(userId, c.GetString(ctxkey.RequestId), quota); err != nil {
			lg.Error("update user request cost failed", zap.Error(err))
		}
	}()

	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)

	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
	}
	return nil
}

// audioRollbackGateForTest, when non-nil, blocks the rollback goroutine spawned by
// goAudioRollbackPreConsumed until the channel is closed. audioRollbackObservedCtxErrForTest,
// when non-nil, records the context error observed by the rollback goroutine before it
// performs the refund DB write. Both are test seams to verify the rollback goroutine runs
// on a non-cancelled context after the request context is cancelled; they are always nil in
// production builds.
var audioRollbackGateForTest chan struct{}
var audioRollbackObservedCtxErrForTest func(error)

// goAudioRollbackPreConsumed refunds the pre-consumed quota of a failed audio request.
// It delegates to the shared goRollbackPreConsumed, which runs on a detached, c-free
// context (see that function for why). The test seams are snapshotted here on the
// request goroutine and passed by value.
func goAudioRollbackPreConsumed(c *gin.Context, tokenId int, preConsumedQuota int64) {
	goRollbackPreConsumed(c, "audioRollbackPreConsumed", tokenId, preConsumedQuota,
		audioRollbackGateForTest, audioRollbackObservedCtxErrForTest)
}

func getTextFromVTT(body []byte) (string, error) {
	return getTextFromSRT(body)
}

func getTextFromVerboseJSON(body []byte) (string, error) {
	var whisperResponse openai.WhisperVerboseJSONResponse
	if err := json.Unmarshal(body, &whisperResponse); err != nil {
		return "", errors.Wrap(err, "unmarshal_response_body_failed")
	}

	return whisperResponse.Text, nil
}

func getTextFromSRT(body []byte) (string, error) {
	var builder strings.Builder
	var textLine bool
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if textLine {
			builder.WriteString(line)
			textLine = false
			continue
		} else if strings.Contains(line, "-->") {
			textLine = true
			continue
		}
	}
	return builder.String(), nil
}

func getTextFromText(body []byte) (string, error) {
	return strings.TrimSuffix(string(body), "\n"), nil
}

func getTextFromJSON(body []byte) (string, error) {
	var whisperResponse openai.WhisperJSONResponse
	if err := json.Unmarshal(body, &whisperResponse); err != nil {
		return "", errors.Wrap(err, "unmarshal_response_body_failed")
	}
	return whisperResponse.Text, nil
}
