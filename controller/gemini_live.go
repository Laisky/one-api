package controller

import (
	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"math"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// isGeminiLiveRequest selects the native wire protocol by channel, not by a
// client-controlled model prefix. Parameters: m is mapped metadata. Returns:
// true for Developer API Gemini channels, including the compatibility channel.
func isGeminiLiveRequest(m *meta.Meta) bool { return m != nil && gemini.IsLiveChannel(m.ChannelType) }

// validateGeminiRealtimeTransport rejects unsupported Google sessions before reserving
// quota or dialing. Parameters: m is authenticated metadata. Returns: a safe
// error, or nil when the existing provider handler may proceed.
func validateGeminiRealtimeTransport(m *meta.Meta) error {
	if m == nil {
		return errors.WithStack(gemini.ErrLiveProtocol)
	}
	if isGeminiLiveRequest(m) {
		if _, err := gemini.LiveRequestURL(m); err != nil {
			return err
		}
		if m.APIKey == "" {
			return errors.Wrap(gemini.ErrLiveProtocol, "missing channel API key")
		}
	}
	if m.ChannelType == channeltype.VertextAI && realtime.IsGeminiLiveModel(m.ActualModelName) {
		return errors.Wrap(gemini.ErrLiveProtocol, "Vertex Live requires a separate transport and pricing configuration")
	}
	return nil
}

// runRealtimeProviderWithGemini dispatches the session without changing OpenAI or Zhipu
// behavior. Parameters: c is the authenticated request and m its metadata.
// Returns: a handshake error or final provider-specific usage.
func runRealtimeProviderWithGemini(c *gin.Context, m *meta.Meta) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	if isGeminiLiveRequest(m) {
		return gemini.LiveHandler(c, m)
	}
	if m.APIType == apitype.Zhipu {
		return zhipu.RealtimeHandler(c, m)
	}
	return openai.RealtimeHandler(c, m)
}

// estimateRealtimeSessionReservation uses Gemini's documented 25 audio tokens
// per second in both directions, rather than OpenAI's 10/20 rates. Parameters:
// m and pricing arguments describe the bound session. Returns: a reservation,
// not a minimum fee; final settlement always uses provider usage receipts.
func estimateRealtimeSessionReservation(m *meta.Meta, modelRatio, groupRatio float64, configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor) (int64, error) {
	if !isGeminiLiveRequest(m) {
		return estimateRealtimePreConsumeQuota(m.ActualModelName, modelRatio, groupRatio, configs, provider, m.StartTime), nil
	}
	if modelRatio < 0 || groupRatio < 0 || math.IsNaN(modelRatio) || math.IsNaN(groupRatio) || math.IsInf(modelRatio, 0) || math.IsInf(groupRatio, 0) {
		return 0, errors.Wrap(realtime.ErrInvalidPrice, "invalid Gemini Live reservation pricing")
	}
	tokens := int64(realtimePreConsumeSeconds * 25)
	ledger := realtime.NewLedger()
	ledger.Records = []realtime.Record{{Tokens: realtime.Tokens{Input: tokens, Audio: tokens, Output: tokens, OutputAudio: tokens}}}
	ledger.InputTokens, ledger.OutputTokens = tokens, tokens
	result := quota.Compute(quota.ComputeInput{Usage: &relaymodel.Usage{Realtime: ledger}, ModelName: m.ActualModelName, ModelRatio: modelRatio, GroupRatio: groupRatio, ChannelModelConfigs: configs, PricingAdaptor: provider, RequestTime: m.StartTime})
	if result.UnpricedUsage {
		return 0, errors.Wrap(realtime.ErrInvalidPrice, "unpriceable Gemini Live reservation")
	}
	return result.TotalQuota, nil
}

// geminiLivePricingAdaptor returns the shared Google catalog for both Developer
// channel types. Parameters: none. Returns: the pricing-only native adaptor.
func geminiLivePricingAdaptor() adaptor.Adaptor { return &gemini.Adaptor{} }
