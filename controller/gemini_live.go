package controller

import (
	"math"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// isGeminiLiveRequest selects native framing by channel, not by a caller-chosen
// model prefix. Parameters: m is mapped metadata. Returns: true for Developer
// API and Vertex channels, which retain separate authentication and prices.
func isGeminiLiveRequest(m *meta.Meta) bool {
	return m != nil && (gemini.IsLiveChannel(m.ChannelType) || m.ChannelType == channeltype.VertextAI)
}

// validateGeminiRealtimeTransport validates local configuration before reserving
// quota. Parameters: m is authenticated metadata. Returns: a safe error or nil.
// No release-stage, IAM, region-availability, or model-catalog lookup is performed.
func validateGeminiRealtimeTransport(m *meta.Meta) error {
	if m == nil {
		return errors.WithStack(gemini.ErrLiveProtocol)
	}
	if m.ChannelType == channeltype.VertextAI {
		if _, err := vertexai.LiveRequestURL(m); err != nil {
			return err
		}
		if strings.TrimSpace(m.Config.VertexAIADC) == "" {
			return errors.Wrap(gemini.ErrLiveProtocol, "missing Vertex channel credentials")
		}
		return nil
	}
	if gemini.IsLiveChannel(m.ChannelType) {
		if _, err := gemini.LiveRequestURL(m); err != nil {
			return err
		}
		if m.APIKey == "" {
			return errors.Wrap(gemini.ErrLiveProtocol, "missing channel API key")
		}
	}
	return nil
}

// runRealtimeProviderWithGemini dispatches native sessions without changing the
// OpenAI or Zhipu paths. Parameters: c and m identify the authenticated request.
// Returns: a pre-upgrade error or the joined provider-specific usage ledger.
func runRealtimeProviderWithGemini(c *gin.Context, m *meta.Meta) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	if m.ChannelType == channeltype.VertextAI {
		return vertexai.LiveHandler(c, m)
	}
	if gemini.IsLiveChannel(m.ChannelType) {
		return gemini.LiveHandler(c, m)
	}
	if m.APIType == apitype.Zhipu {
		return zhipu.RealtimeHandler(c, m)
	}
	return openai.RealtimeHandler(c, m)
}

// estimateRealtimeSessionReservation uses the native 25 audio tokens/second
// allowance in both directions. Parameters: m and pricing arguments describe
// the configured session. Returns: a reservation, not a minimum session fee.
// Vertex and unlisted models require administrator prices before provider work.
func estimateRealtimeSessionReservation(m *meta.Meta, modelRatio, groupRatio float64, configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor) (int64, error) {
	if !isGeminiLiveRequest(m) {
		return estimateRealtimePreConsumeQuota(m.ActualModelName, modelRatio, groupRatio, configs, provider, m.StartTime), nil
	}
	if modelRatio < 0 || groupRatio < 0 || math.IsNaN(modelRatio) || math.IsNaN(groupRatio) || math.IsInf(modelRatio, 0) || math.IsInf(groupRatio, 0) {
		return 0, errors.Wrap(realtime.ErrInvalidPrice, "invalid Gemini Live reservation pricing")
	}
	if err := quota.ValidateRealtimeModelPricing(m.ActualModelName, configs, provider, m.StartTime); err != nil {
		return 0, err
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

// geminiLivePricingAdaptor selects a backend-specific pricing policy.
// Parameters: metadata optionally identifies the channel; omitted metadata keeps
// the Developer API helper contract. Returns: the selected pricing adaptor.
func geminiLivePricingAdaptor(metadata ...*meta.Meta) adaptor.Adaptor {
	if len(metadata) > 0 && metadata[0] != nil && metadata[0].ChannelType == channeltype.VertextAI {
		return &vertexai.Adaptor{}
	}
	return &gemini.Adaptor{}
}
