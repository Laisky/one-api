package controller

import (
	"net/http"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/middleware"
	relay "github.com/Laisky/one-api/relay"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/openrouterprovider"
	relaypricing "github.com/Laisky/one-api/relay/pricing"
)

// OpenRouterListModels serves GET /openrouter/v1/models in the OpenRouter
// upstream-provider listing schema. It walks every adaptor registered with
// the relay layer (plus the OpenAI-compatible channels) and emits one entry
// per advertised model with its pricing/metadata. The endpoint is public
// because OpenRouter scrapes it during onboarding and periodic refresh.
func OpenRouterListModels(c *gin.Context) {
	lg := gmw.GetLogger(c)

	inputs, err := collectOpenRouterModelInputs()
	if err != nil {
		lg.Error("collect openrouter model inputs failed", zap.Error(err))
		middleware.AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "collect openrouter model inputs"))
		return
	}

	response := openrouterprovider.BuildModelListResponse(inputs)
	c.JSON(http.StatusOK, response)
}

// collectOpenRouterModelInputs builds the deduplicated catalog of
// (model name, ModelConfig, owner) tuples that feeds the OpenRouter listing.
// It mirrors the aggregation performed at controller package init time, with
// two important differences: it returns the per-model adaptor.ModelConfig (not
// just the OpenAI-compatible permission record), and it deduplicates by
// case-insensitive model id keeping the first occurrence so the catalog
// remains stable across calls. Each entry quotes the tariff billing applies at
// listing time (relaypricing.ApplyTimeWindow), the same contract as the
// top-level prices of /api/models/display, because the OpenRouter schema has
// no field for a dated or time-of-day schedule.
func collectOpenRouterModelInputs() ([]openrouterprovider.ModelInput, error) {
	at := modelsDisplayNow()
	created := at.Unix()
	seen := make(map[string]struct{})
	inputs := make([]openrouterprovider.ModelInput, 0, 128)

	appendEntry := func(modelName, owner string, cfg adaptorpkg.ModelConfig) {
		trimmed := strings.TrimSpace(modelName)
		if trimmed == "" {
			return
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		inputs = append(inputs, openrouterprovider.ModelInput{
			Name:    trimmed,
			Owner:   strings.TrimSpace(owner),
			Config:  relaypricing.ApplyTimeWindow(cfg, at),
			Created: created,
		})
	}

	// Primary adaptor sweep: walk every API type once and pull pricing
	// metadata directly from the adaptor implementation.
	for apiType := 0; apiType < apitype.Dummy; apiType++ {
		if apiType == apitype.AIProxyLibrary {
			continue
		}
		adp := relay.GetAdaptor(apiType)
		if adp == nil {
			continue
		}
		// Price each adaptor as the channel type that routes to it would be
		// billed, so first-party rules (such as native OpenAI's dated
		// schedules) apply exactly where billing applies them.
		adp.Init(&meta.Meta{ChannelType: canonicalChannelType(apiType)})
		owner := adp.GetChannelName()
		pricing := adp.GetDefaultModelPricing()
		for _, modelName := range adp.GetModelList() {
			cfg := pricing[modelName]
			appendEntry(modelName, owner, cfg)
		}
	}

	// OpenAI-compatible channels piggy-back on the OpenAI adaptor but expose
	// distinct model lists per upstream provider. Walk them separately so
	// each provider gets its own owner label.
	for _, channelType := range openai.CompatibleChannels {
		if channelType == channeltype.Azure {
			continue
		}
		channelName, channelModels := openai.GetCompatibleChannelMeta(channelType)
		adp := &openai.Adaptor{}
		adp.Init(&meta.Meta{ChannelType: channelType})
		pricing := adp.GetDefaultModelPricing()
		for _, modelName := range channelModels {
			cfg := pricing[modelName]
			appendEntry(modelName, channelName, cfg)
		}
	}

	sort.Slice(inputs, func(i, j int) bool {
		return strings.ToLower(inputs[i].Name) < strings.ToLower(inputs[j].Name)
	})
	return inputs, nil
}

// canonicalChannelType returns the first channel type whose requests route to
// apiType, which is the provider's own channel type for every first-party
// adaptor (channeltype.OpenAI for apitype.OpenAI). Parameters: apiType is a
// relay/apitype value. Returns: the channel type, or 0 when none routes there.
func canonicalChannelType(apiType int) int {
	for channelType := channeltype.Unknown + 1; channelType < channeltype.Dummy; channelType++ {
		if channeltype.ToAPIType(channelType) == apiType {
			return channelType
		}
	}
	return channeltype.Unknown
}
