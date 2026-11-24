package controller

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
)

// zeroConfigServableModels returns the models a new channel of channelType may
// be filled with by default: the adaptor's model list minus every model request
// admission would refuse while the channel's model_configs are empty (models
// the host serves without a qualified tariff, or Live-only models whose host
// requires operator realtime prices). Such models stay in the catalog, so an
// operator can still add and price them; they are only kept out of the default
// list, so an unconfigured admin never fills, resets, or saves a channel into
// pricing warnings or refused requests. Parameters: channelType is a channel
// type ID and models its adaptor's model list. Returns: the filtered list in
// the original order, never sharing models' backing array.
func zeroConfigServableModels(channelType int, models []string) []string {
	provider := relaycontroller.ResolveChannelPricingAdaptor(channelType)
	if provider == nil || len(models) == 0 {
		return slices.Clone(models)
	}
	gaps := relaycontroller.EvaluateChannelPricingReadiness(provider, models, nil, nil, time.Now().UTC())
	refused := make(map[string]struct{}, len(gaps))
	for _, gap := range gaps {
		refused[gap.Model] = struct{}{}
	}
	servable := make([]string, 0, len(models)-len(refused))
	for _, name := range models {
		if _, ok := refused[name]; !ok {
			servable = append(servable, name)
		}
	}
	return servable
}

// DashboardListDefaultModels returns the zero-config recommendations per
// channel type, in the same shape as DashboardListModels: the models a new
// channel is filled with and a reset writes, which are servable without
// operator pricing. Models absent here but present in GET /api/models remain
// addable once the operator prices them. Parameters: c is the admin request.
// Returns: none; it writes {"success":true,"message":"","data":{...}}.
func DashboardListDefaultModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channelId2DefaultModels,
	})
}

// explicitlyServableModels reports which candidate models a channel's own
// configuration makes servable: those that request admission would not refuse
// for missing pricing under the channel's model_mapping and model_configs,
// evaluated with the admission validators. It reads no rows and changes
// nothing. Parameters: ctx carries the request logger, channel is the locked
// channel row, and candidates are catalog models outside the defaults.
// Returns: the servable membership map (nil when the channel type has no
// pricing adaptor or there are no candidates), or a wrapped parse error.
func explicitlyServableModels(ctx context.Context, channel *model.Channel, candidates []string) (map[string]bool, error) {
	if channel == nil || len(candidates) == 0 {
		return nil, nil
	}
	provider := relaycontroller.ResolveChannelPricingAdaptor(channel.Type)
	if provider == nil {
		return nil, nil
	}
	mapping, err := model.ParseModelMapping(channel.ModelMapping)
	if err != nil {
		return nil, errors.Wrap(err, "parse channel model mapping for reset servability")
	}
	configs, err := model.ParseModelPriceConfigs(channel.ModelConfigs)
	if err != nil {
		return nil, errors.Wrap(err, "parse channel model configs for reset servability")
	}
	gaps := relaycontroller.EvaluateChannelPricingReadiness(provider, candidates, mapping, configs, time.Now().UTC())
	refused := make(map[string]bool, len(gaps))
	for _, gap := range gaps {
		refused[gap.Model] = true
	}
	servable := make(map[string]bool, len(candidates))
	for _, name := range candidates {
		if !refused[name] {
			servable[name] = true
		}
	}
	logger.FromContext(ctx).Debug("evaluated reset retention candidates",
		append(channel.Ref().Zap(),
			zap.Strings("candidates", candidates),
			zap.Int("servable", len(servable)),
			zap.Int("pricing_gaps", len(gaps)))...)
	return servable, nil
}
