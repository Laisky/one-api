package controller

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const nativeGoogleMediaUsageKey = "oneapi.native_google_media_prompt_usage"
const nativeGooglePreparedKey = "oneapi.native_google_prepared_request"

type nativeGooglePrepared struct {
	source  *relaymodel.GeneralOpenAIRequest
	payload *gemini.ChatRequest
	model   string
	channel int
	apiType int
}

// isNativeGoogleChannel scopes media admission to actual native Google routes.
// Vertex partner models and generic OpenAI-compatible channels are not changed.
func isNativeGoogleChannel(meta *metalib.Meta) bool {
	return meta != nil && (meta.APIType == apitype.Gemini ||
		(meta.APIType == apitype.VertexAI && strings.HasPrefix(strings.ToLower(meta.ActualModelName), "gemini-")))
}

// preparedGoogleOrConvertRequest reuses the exact native payload counted before
// admission. Other providers retain their existing conversion path.
func preparedGoogleOrConvertRequest(c *gin.Context, meta *metalib.Meta, request *relaymodel.GeneralOpenAIRequest, provider adaptor.Adaptor) (any, error) {
	if isNativeGoogleChannel(meta) {
		if raw, exists := c.Get(nativeGooglePreparedKey); exists {
			if cached, ok := raw.(nativeGooglePrepared); ok && cached.source == request && cached.model == meta.ActualModelName && cached.channel == meta.ChannelId && cached.apiType == meta.APIType {
				return cached.payload, nil
			}
		}
	}
	return provider.ConvertRequest(c, relaymode.ChatCompletions, request)
}

// estimateNativeGooglePromptUsage prepares native Google payloads for the Chat
// and Responses entrypoints, counting video/audio/PDF rather than text alone.
// handled=false preserves the legacy estimate for text and existing image input.
func estimateNativeGooglePromptUsage(c *gin.Context, meta *metalib.Meta, request *relaymodel.GeneralOpenAIRequest) (*relaymodel.Usage, bool, *relaymodel.ErrorWithStatusCode) {
	if !isNativeGoogleChannel(meta) || meta.Mode != relaymode.ChatCompletions {
		return nil, false, nil
	}
	c.Set(nativeGoogleMediaUsageKey, nil)
	c.Set(gemini.NativeUsageReceiptKey, nil)
	provider := relay.GetAdaptor(meta.APIType)
	if provider == nil {
		return nil, true, openai.ErrorWrapper(errors.New("native media channel is unavailable"), "invalid_api_type", http.StatusBadRequest)
	}
	provider.Init(meta)
	// Ordinary Chat already prepared its exact payload in getRequestBody.
	// Responses prepares here and binds the result to this request/channel.
	var converted any
	var err error
	if !meta.ResponseAPIFallback {
		converted, _ = c.Get(ctxkey.ConvertedRequest)
	}
	if _, ok := converted.(*gemini.ChatRequest); !ok {
		converted, err = provider.ConvertRequest(c, relaymode.ChatCompletions, request)
	}
	if err != nil {
		return nil, true, wrapConvertRequestError(err)
	}
	c.Set(ctxkey.ConvertedRequest, converted)
	prepared, ok := converted.(*gemini.ChatRequest)
	if ok && prepared != nil {
		c.Set(nativeGooglePreparedKey, nativeGooglePrepared{request, prepared, meta.ActualModelName, meta.ChannelId, meta.APIType})
	}
	if !ok || !gemini.HasExtendedMedia(prepared) {
		return nil, false, nil
	}
	usage, err := gemini.CountNativeChatTokens(c, meta, prepared, provider)
	if err != nil {
		status := http.StatusBadGateway
		if shouldTreatConvertRequestErrorAsBadRequest(err) {
			status = http.StatusBadRequest
		}
		return nil, true, openai.ErrorWrapper(err, "native_media_prompt_count_failed", status)
	}
	c.Set(nativeGoogleMediaUsageKey, usage)
	return usage, true, nil
}

// preparedClaudePromptTokensForChannel brings Claude's native Google route under
// the same media admission contract without altering Anthropic/partner pricing.
func preparedClaudePromptTokensForChannel(c *gin.Context, meta *metalib.Meta, request *ClaudeMessagesRequest, converted any) (int, error) {
	c.Set(nativeGoogleMediaUsageKey, nil)
	c.Set(gemini.NativeUsageReceiptKey, nil)
	prepared, ok := converted.(*gemini.ChatRequest)
	if !isNativeGoogleChannel(meta) || !ok || !gemini.HasExtendedMedia(prepared) {
		return preparedClaudePromptTokens(gmw.Ctx(c), request, converted)
	}
	provider := relay.GetAdaptor(meta.APIType)
	if provider == nil {
		return 0, errors.New("native media channel is unavailable")
	}
	provider.Init(meta)
	usage, err := gemini.CountNativeChatTokens(c, meta, prepared, provider)
	if err != nil {
		return 0, err
	}
	c.Set(nativeGoogleMediaUsageKey, usage)
	return usage.PromptTokens, nil
}

// nativeGooglePromptErrorStatus preserves client validation errors while
// distinguishing a failed provider preflight from an invalid user request.
func nativeGooglePromptErrorStatus(meta *metalib.Meta, converted any, err error) int {
	prepared, ok := converted.(*gemini.ChatRequest)
	if isNativeGoogleChannel(meta) && ok && gemini.HasExtendedMedia(prepared) && !shouldTreatConvertRequestErrorAsBadRequest(err) {
		return http.StatusBadGateway
	}
	return http.StatusBadRequest
}

// applyNativeGoogleUsageEvidence restores private accounting evidence after a
// public protocol conversion. Missing audio breakdowns retain the existing hold
// explicitly as an estimate instead of refunding down to the cheaper text rate.
func applyNativeGoogleUsageEvidence(c *gin.Context, usage *relaymodel.Usage) {
	if usage == nil {
		return
	}
	if raw, ok := c.Get(gemini.NativeUsageReceiptKey); ok {
		if receipt, valid := raw.(*relaymodel.Usage); valid && receipt != nil && receipt.PromptTokens == usage.PromptTokens {
			usage.NativeGoogleAudio = receipt.NativeGoogleAudio
			usage.PromptTokensDetails = receipt.PromptTokensDetails
			if receipt.BillingEstimateReason != "" {
				usage.BillingEstimateReason = receipt.BillingEstimateReason
			}
		}
	}
	if raw, ok := c.Get(nativeGoogleMediaUsageKey); ok {
		if expected, valid := raw.(*relaymodel.Usage); valid && expected != nil && expected.NativeGoogleAudio != nil && expected.NativeGoogleAudio.Tokens > 0 && usage.NativeGoogleAudio == nil {
			usage.BillingEstimateReason = "gemini_audio_usage_missing_modality_receipt"
		}
	}
}
