package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

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
	"github.com/Laisky/one-api/relay/adaptor/gemini/tts"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// isGeminiSpeechChannel selects native Google transports without affecting third-party bridges.
func isGeminiSpeechChannel(channel int) bool {
	return channel == channeltype.Gemini || channel == channeltype.GeminiOpenAICompatible || channel == channeltype.VertextAI
}

// geminiSpeechURL resolves a native endpoint while keeping administrator path prefixes intact.
func geminiSpeechURL(m *meta.Meta, streaming bool) (string, error) {
	if !tts.SupportsModel(m.ActualModelName) {
		return "", errors.New("unsupported Gemini speech model")
	}
	override := m.UpstreamEndpointURLOverride()
	if override == "" && m.ChannelType == channeltype.VertextAI {
		if m.Config.VertexAIProjectID == "" {
			return "", errors.New("Vertex speech requires a project ID")
		}
		copy := *m
		copy.IsStream = streaming
		copy.Mode = relaymode.AudioSpeech
		value, err := (&vertexai.Adaptor{}).GetRequestURL(&copy)
		if err != nil {
			return "", errors.Wrap(err, "resolve Vertex speech endpoint")
		}
		override = value
	}
	base := m.BaseURL
	if base == "" {
		base = channeltype.ChannelBaseURLs[m.ChannelType]
	}
	if override != "" {
		base = override
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", errors.New("invalid Gemini speech upstream URL")
	}
	query := parsed.Query()
	for key := range query {
		if key != "alt" {
			return "", errors.New("speech upstream credentials and options must not be placed in URL queries")
		}
	}
	if override == "" {
		path := strings.TrimSuffix(strings.TrimRight(parsed.Path, "/"), "/openai")
		version := "v1beta"
		for _, candidate := range []string{"v1beta", "v1alpha", "v1"} {
			if strings.HasSuffix(path, "/"+candidate) {
				version = candidate
				path = strings.TrimSuffix(path, "/"+candidate)
				break
			}
		}
		if m.Config.APIVersion != "" {
			version = m.Config.APIVersion
		}
		if version != "v1beta" && version != "v1alpha" && version != "v1" {
			return "", errors.New("invalid Gemini speech API version")
		}
		action := "generateContent"
		if streaming {
			action = "streamGenerateContent"
		}
		parsed.Path = path + "/" + version + "/models/" + m.ActualModelName + ":" + action
		parsed.RawPath = ""
	}
	if streaming {
		query.Set("alt", "sse")
	} else {
		query.Del("alt")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// relayGeminiSpeech converts and settles one speech attempt using the shared audio ledger.
// All validation precedes reservation; accepted audio is charged even on client disconnect.
func relayGeminiSpeech(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	ctx, cancel := context.WithCancel(gmw.Ctx(c))
	stopCancellation := context.AfterFunc(c.Request.Context(), cancel)
	defer func() { stopCancellation(); cancel() }()
	lg := gmw.GetLogger(c)
	m := meta.GetByContext(c)
	var request openai.TextToSpeechRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return openai.ErrorWrapper(errors.New("invalid speech JSON"), "invalid_audio_request", http.StatusBadRequest)
	}
	m.OriginModelName = request.Model
	m.ActualModelName = meta.GetMappedModelName(request.Model, c.GetStringMapString(ctxkey.ModelMapping))
	m.Mode = relaymode.AudioSpeech
	body, _, err := normalizeAudioWire(c, relaymode.AudioSpeech, m.ChannelType, m.ActualModelName, &request)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_audio_request", http.StatusBadRequest)
	}
	plan, err := tts.Prepare(body, m.ActualModelName)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_audio_request", http.StatusBadRequest)
	}
	m.IsStream = plan.Stream
	target, err := geminiSpeechURL(m, plan.Stream)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_speech_channel", http.StatusBadRequest)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(plan.Body))
	if err != nil {
		return openai.ErrorWrapper(errors.New("could not create speech request"), "invalid_speech_channel", http.StatusInternalServerError)
	}
	prices, err := loadGeminiSpeechPrices(c, m)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_audio_pricing", http.StatusBadRequest)
	}
	reservation, err := prices.reserve(plan.OutputLimit)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_audio_pricing", http.StatusBadRequest)
	}
	if m.ChannelType == channeltype.VertextAI {
		if err := (&vertexai.Adaptor{}).SetupRequestHeader(c, req, m); err != nil {
			return openai.ErrorWrapper(errors.New("could not authenticate Vertex speech channel"), "speech_auth_failed", http.StatusInternalServerError)
		}
	} else {
		if m.APIKey == "" {
			return openai.ErrorWrapper(errors.New("Gemini speech channel key is missing"), "speech_auth_failed", http.StatusInternalServerError)
		}
		req.Header.Set("x-goog-api-key", m.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if plan.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	userID, tokenID := c.GetInt(ctxkey.Id), c.GetInt(ctxkey.TokenId)
	balance, err := model.CacheGetUserQuota(ctx, userID)
	if err != nil {
		return openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if balance < reservation {
		return openai.ErrorWrapper(errors.New("user quota is not enough for the speech output budget"), "insufficient_user_quota", http.StatusForbidden)
	}
	prepaid := reservation
	if prepaid < balance/100 && (c.GetBool(ctxkey.TokenQuotaUnlimited) || prepaid < c.GetInt64(ctxkey.TokenQuota)/100) {
		prepaid = 0
	}
	if prepaid > 0 {
		if err := model.PreConsumeTokenQuota(ctx, tokenID, prepaid); err != nil {
			return openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
		syncUserQuotaCacheAfterPreConsume(ctx, userID, prepaid, "gemini_speech_preconsume")
		markPreConsumed(c, prepaid)
		defer billingAuditSafetyNet(c)
		c.Set(ctxkey.ProvisionalLogId, recordProvisionalLog(c, m, request.Model, prepaid))
	}
	var receipt tts.Receipt
	defer func() { settleGeminiSpeech(c, m, prices, receipt, prepaid) }()
	// Do not log the transcript, style, upstream response body, or voicekey_ credentials.
	lg.Debug("sending Gemini speech request", zap.String("model", m.ActualModelName), zap.String("format", plan.Format), zap.Bool("stream", plan.Stream))
	m.UpstreamRequestURL = target
	c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
	// A redirect could leak a Google key to a different host. Do not follow it.
	httpClient := *client.HTTPClient
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := httpClient.Do(req)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "Gemini speech request failed"), "do_request_failed", http.StatusBadGateway)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			lg.Warn("close Gemini speech response", zap.Error(err))
		}
	}()
	if err := model.UpdateUserRequestCostQuotaByRequestID(userID, c.GetString(ctxkey.RequestId), reservation); err != nil {
		lg.Warn("record provisional speech cost", zap.Error(err))
	}
	if resp.StatusCode != http.StatusOK {
		status := resp.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		return openai.ErrorWrapper(errors.New("Gemini speech upstream rejected the request"), "gemini_speech_upstream_error", status)
	}
	receipt, err = plan.Forward(ctx, resp, c.Writer, func(accepted tts.Receipt) {
		receipt = accepted
		c.Set(adaptor.AudioReceiptAcceptedKey, true)
	})
	if err != nil {
		return openai.ErrorWrapper(err, "gemini_speech_response_failed", http.StatusBadGateway)
	}
	return nil
}

// settleGeminiSpeech reconciles one immutable receipt, with detached lifecycle-tracked writes.
func settleGeminiSpeech(c *gin.Context, m *meta.Meta, prices *geminiSpeechPrices, receipt tts.Receipt, prepaid int64) {
	lg := gmw.GetLogger(c)
	userID, tokenID, logID := c.GetInt(ctxkey.Id), c.GetInt(ctxkey.TokenId), c.GetInt(ctxkey.ProvisionalLogId)
	requestID := c.GetString(ctxkey.RequestId)
	if !receipt.Accepted {
		if logID > 0 {
			if err := model.ReconcileConsumeLog(detachForBilling(c), logID, 0, "Gemini speech failed before audio receipt, refunded", 0, 0, 0, nil); err != nil {
				lg.Warn("reconcile rejected speech log", zap.Error(err))
			}
		}
		if err := model.UpdateUserRequestCostQuotaByRequestID(userID, requestID, 0); err != nil {
			lg.Warn("reconcile rejected speech cost", zap.Error(err))
		}
		markBillingReconciled(c)
		if prepaid > 0 {
			goAudioRollbackPreConsumed(c, tokenID, prepaid)
		}
		return
	}
	c.Set(adaptor.AudioReceiptAcceptedKey, true)
	total, err := prices.charge(receipt)
	if err != nil {
		// Keep the reservation unresolved for billingAuditSafetyNet; never refund accepted work.
		lg.Error("Gemini speech receipt needs billing reconciliation", zap.Error(err))
		return
	}
	var traceID string
	if id, err := gmw.TraceID(c); err == nil {
		traceID = id.String()
	}
	entry := &model.Log{
		UserId: userID, UserUUID: model.StringPtrIfNotEmpty(m.UserUUID),
		ChannelId: m.ChannelId, ChannelUUID: model.StringPtrIfNotEmpty(m.ChannelUUID),
		TokenName: c.GetString(ctxkey.TokenName), TokenUUID: model.StringPtrIfNotEmpty(m.TokenUUID),
		PromptTokens: receipt.PromptTokens, CompletionTokens: receipt.OutputTokens,
		ModelName: m.OriginModelName, RequestId: requestID, TraceId: traceID,
		ElapsedTime: helper.CalcElapsedTime(m.StartTime),
		Content:     fmt.Sprintf("Gemini speech token billing: cached_input=%d usage_complete=%t output_estimated_from_pcm=%t pcm_bytes=%d", receipt.CachedTokens, receipt.UsageComplete, receipt.EstimatedOutput, receipt.AudioBytes),
	}
	if !receipt.UsageComplete {
		lg.Warn("Gemini speech usage is incomplete; recording received-audio estimate", zap.Int("pcm_bytes", receipt.AudioBytes), zap.Int("prompt_tokens", receipt.PromptTokens), zap.Int("output_tokens", receipt.OutputTokens))
	}
	bg := detachForBilling(c)
	graceful.GoCritical(bg, "geminiSpeechPostConsume", func(ctx context.Context) {
		// Create the timeout inside the task; cancelling it on request return would abort settlement.
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		billing.PostConsumeQuotaWithLog(ctx, tokenID, total-prepaid, total, entry, logID)
	})
	if err := model.UpdateUserRequestCostQuotaByRequestID(userID, requestID, total); err != nil {
		lg.Error("update Gemini speech request cost", zap.Error(err))
	}
	markBillingReconciled(c)
}
