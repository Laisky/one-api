package controller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/relaymode"
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
		if len(ttsRequest.Input) > 4096 {
			return openai.ErrorWrapper(errors.New("input is too long (over 4096 characters)"), "text_too_long", http.StatusBadRequest)
		}
	} else if relayMode == relaymode.AudioTranscription || relayMode == relaymode.AudioTranslation {
		// Extract `model` from multipart form for transcription/translation
		if m := extractAudioModelFromMultipart(c); m != "" {
			audioModel = m
		}
	}

	// get channel-specific pricing if available
	var channelModelRatio map[string]float64
	var channelModelConfigs map[string]model.ModelConfigLocal
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			// Get from unified ModelConfigs only (after migration)
			channelModelRatio = channel.GetModelRatioFromConfigs()
			channelModelConfigs = channel.GetModelPriceConfigs()
		}
	}

	// Use three-layer pricing system
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.GetModelRatioWithThreeLayers(audioModel, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	ratio := modelRatio * groupRatio

	audioPricingCfg, hasAudioPricing := pricing.ResolveAudioPricing(audioModel, channelModelConfigs, pricingAdaptor)
	tokensPerSecond := pricing.DefaultAudioPromptTokensPerSecond
	if hasAudioPricing && audioPricingCfg != nil && audioPricingCfg.PromptTokensPerSecond > 0 {
		tokensPerSecond = audioPricingCfg.PromptTokensPerSecond
	}
	var quota int64
	var preConsumedQuota int64
	switch relayMode {
	case relaymode.AudioSpeech:
		preConsumedQuota = int64(float64(len(ttsRequest.Input)) * ratio)
		quota = preConsumedQuota
	case relaymode.AudioTranscription,
		relaymode.AudioTranslation:
		audioTokens, err := countAudioTokens(c, tokensPerSecond)
		if err != nil {
			return openai.ErrorWrapper(err, "count_audio_tokens_failed", http.StatusInternalServerError)
		}

		preConsumedQuota = int64(math.Ceil(audioTokens * ratio))
		quota = preConsumedQuota
	default:
		return openai.ErrorWrapper(errors.New("unexpected_relay_mode"), "unexpected_relay_mode", http.StatusInternalServerError)
	}

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
	err = model.CacheDecreaseUserQuota(ctx, userId, preConsumedQuota)
	if err != nil {
		return openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota > 100*preConsumedQuota &&
		(tokenQuotaUnlimited || tokenQuota > 100*preConsumedQuota) {
		// in this case, we do not pre-consume quota
		// because the user has enough quota
		preConsumedQuota = 0
	}
	if preConsumedQuota > 0 {
		err := model.PreConsumeTokenQuota(ctx, tokenId, preConsumedQuota)
		if err != nil {
			return openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
	}
	succeed := false
	defer func() {
		if succeed {
			return
		}
		if preConsumedQuota > 0 {
			// we need to roll back the pre-consumed quota under lifecycle tracking
			defer func() {
				graceful.GoCritical(ctx, "audioRollbackPreConsumed", func(cctx context.Context) {
					if err := model.PostConsumeTokenQuota(cctx, tokenId, -preConsumedQuota); err != nil {
						gmw.GetLogger(cctx).Error("error rollback pre-consumed quota", zap.Error(err))
					}
				})
			}()
		}
	}()

	// map model name
	modelMapping := c.GetStringMapString(ctxkey.ModelMapping)
	if modelMapping != nil && modelMapping[audioModel] != "" {
		audioModel = modelMapping[audioModel]
	}

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

	// Reconstruct the original request body from cache to ensure full payload is forwarded
	rawBody, err := common.GetRequestBody(c)
	if err != nil {
		return openai.ErrorWrapper(err, "get_request_body_failed", http.StatusInternalServerError)
	}
	requestBody := bytes.NewBuffer(rawBody)
	// Reset gin Request.Body for any subsequent operations that may need it
	c.Request.Body = io.NopCloser(bytes.NewReader(rawBody))
	// responseFormat := c.DefaultPostForm("response_format", "json")

	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return openai.ErrorWrapper(err, "new_request_failed", http.StatusInternalServerError)
	}

	if (relayMode == relaymode.AudioTranscription || relayMode == relaymode.AudioSpeech) && channelType == channeltype.Azure {
		// https://learn.microsoft.com/en-us/azure/ai-services/openai/whisper-quickstart?tabs=command-line#rest-api
		apiKey := c.Request.Header.Get("Authorization")
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		req.Header.Set("api-key", apiKey)
		req.ContentLength = c.Request.ContentLength
	} else {
		req.Header.Set("Authorization", c.Request.Header.Get("Authorization"))
	}
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))

	lg := gmw.GetLogger(c)
	// Log upstream request for billing tracking
	lg.Info("sending audio request to upstream channel",
		zap.String("url", fullRequestURL),
		zap.Int("channelId", channelId),
		zap.Int("userId", userId),
		zap.String("model", audioModel),
		zap.Int("relayMode", relayMode))

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		// Let ErrorWrapper handle the logging to avoid duplicate logging
		return openai.ErrorWrapper(errors.Wrapf(err, "upstream audio request failed for channel %d", channelId), "do_request_failed", http.StatusInternalServerError)
	}

	// Immediately record a provisional request cost using the estimated quota, even if we skipped physical pre-consume
	// (trusted path). This ensures cancellation cases are still tracked and later reconciled.
	{
		requestId := c.GetString(ctxkey.RequestId)
		if err := model.UpdateUserRequestCostQuotaByRequestID(userId, requestId, quota); err != nil {
			lg.Warn("record provisional user request cost failed", zap.Error(err))
		}
	}

	err = req.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_request_body_failed", http.StatusInternalServerError)
	}
	err = c.Request.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_request_body_failed", http.StatusInternalServerError)
	}

	// https://github.com/Laisky/one-api/pull/21
	// Commenting out the following code because Whisper's transcription
	// only charges for the length of the input audio, not for the output.
	// -------------------------------------
	// if relayMode != relaymode.AudioSpeech {
	// 	responseBody, err := io.ReadAll(resp.Body)
	// 	if err != nil {
	// 		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	// 	}
	// 	err = resp.Body.Close()
	// 	if err != nil {
	// 		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError)
	// 	}

	// 	var openAIErr openai.SlimTextResponse
	// 	if err = json.Unmarshal(responseBody, &openAIErr); err == nil {
	// 		if openAIErr.Error.Message != "" {
	// 			return openai.ErrorWrapper(errors.Errorf("type %s, code %v, message %s", openAIErr.Error.Type, openAIErr.Error.Code, openAIErr.Error.Message), "request_error", http.StatusInternalServerError)
	// 		}
	// 	}

	// 	var text string
	// 	switch responseFormat {
	// 	case "json":
	// 		text, err = getTextFromJSON(responseBody)
	// 	case "text":
	// 		text, err = getTextFromText(responseBody)
	// 	case "srt":
	// 		text, err = getTextFromSRT(responseBody)
	// 	case "verbose_json":
	// 		text, err = getTextFromVerboseJSON(responseBody)
	// 	case "vtt":
	// 		text, err = getTextFromVTT(responseBody)
	// 	default:
	// 		return openai.ErrorWrapper(errors.New("unexpected_response_format"), "unexpected_response_format", http.StatusInternalServerError)
	// 	}
	// 	if err != nil {
	// 		return openai.ErrorWrapper(err, "get_text_from_body_err", http.StatusInternalServerError)
	// 	}
	// 	quota = int64(openai.CountTokenText(text, audioModel))
	// 	resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
	// }

	if resp.StatusCode != http.StatusOK {
		// Reconcile provisional record to 0 since upstream returned error
		if err := model.UpdateUserRequestCostQuotaByRequestID(userId, c.GetString(ctxkey.RequestId), 0); err != nil {
			lg.Warn("update user request cost to zero failed", zap.Error(err))
		}
		return RelayErrorHandler(resp)
	}

	succeed = true
	quotaDelta := quota - preConsumedQuota

	// Capture trace ID from gin context now; the background context will not carry gin
	var traceID string
	if tid, err := gmw.TraceID(c); err == nil {
		traceID = tid.String()
	}

	defer func() {
		bgctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), time.Minute)
		defer cancel()

		// Build a full log entry with IDs from gin.Context
		logContent := fmt.Sprintf("model rate %.2f, group rate %.2f", modelRatio, groupRatio)
		entry := &model.Log{
			UserId:           userId,
			ChannelId:        channelId,
			PromptTokens:     int(quota), // audio API logs total as prompt tokens
			CompletionTokens: 0,
			ModelName:        audioModel,
			TokenName:        tokenName,
			Content:          logContent,
			RequestId:        c.GetString(ctxkey.RequestId),
			TraceId:          traceID,
			ElapsedTime:      helper.CalcElapsedTime(meta.StartTime), // capture request latency in ms
		}
		graceful.GoCritical(bgctx, "audioPostConsumeWithLog", func(cctx context.Context) {
			billing.PostConsumeQuotaWithLog(cctx, tokenId, quotaDelta, quota, entry)
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
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError)
	}
	return nil
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
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	helper.ConfigureScannerBuffer(scanner)
	var builder strings.Builder
	var textLine bool
	for scanner.Scan() {
		line := scanner.Text()
		if textLine {
			builder.WriteString(line)
			textLine = false
			continue
		} else if strings.Contains(line, "-->") {
			textLine = true
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
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
