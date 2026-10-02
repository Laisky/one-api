package controller

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
)

// channelModelResetOutcome reports a reset using public UUIDs and safe metadata,
// without serializing credentials, internal IDs, or pricing/configuration values.
type channelModelResetOutcome struct {
	UUID       string                           `json:"uuid"`
	Name       string                           `json:"name"`
	Success    bool                             `json:"success"`
	ModelCount int                              `json:"model_count,omitempty"`
	Message    string                           `json:"message,omitempty"`
	Conflict   *model.ChannelModelResetConflict `json:"conflict,omitempty"`
}

// channelModelResetSummary distinguishes successful resets, policy rejections,
// and operational failures for an entire server-side channel snapshot.
type channelModelResetSummary struct {
	Total    int                        `json:"total"`
	Reset    int                        `json:"reset"`
	Rejected int                        `json:"rejected"`
	Failed   int                        `json:"failed"`
	Results  []channelModelResetOutcome `json:"results"`
}

// defaultChannelResetCatalog returns the same provider-specific model catalog
// advertised by the channel editor, never the union of every provider's models.
func defaultChannelResetCatalog(channelType int) []string {
	return channelId2Models[channelType]
}

// ResetChannelModels handles an administrator's single-channel reset request.
// The server resolves the UUID and validates the locked, current configuration;
// a policy rejection returns HTTP 409 without changing the channel or abilities.
func ResetChannelModels(c *gin.Context) {
	lg := gmw.GetLogger(c)
	id, err := resolveChannelRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	channel, err := model.ResetChannelModelsToDefaults(gmw.Ctx(c), id, defaultChannelResetCatalog)
	if err != nil {
		var conflict *model.ChannelModelResetConflict
		if errors.As(err, &conflict) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": conflict.Error(), "conflict": conflict})
			return
		}
		if lg != nil {
			lg.Error("channel model reset failed", zap.String("channel_uuid", c.Param("id")), zap.Error(err))
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to reset channel models. No changes were applied."})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": channelModelResetOutcome{
			UUID: channel.UUID, Name: channel.Name, Success: true,
			ModelCount: len(channel.GetSupportedModelNames()),
		},
	})
}

// ResetSelectedChannelModels applies the safety contract to confirmed channel UUIDs only.
// Each channel is atomic; conflicts leave it untouched without blocking eligible selected channels.
func ResetSelectedChannelModels(c *gin.Context) {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	targets, err := selectedChannelTargets(c, true)
	if err != nil {
		respondListSelectionError(c, err)
		return
	}
	summary := channelModelResetSummary{Total: len(targets), Results: make([]channelModelResetOutcome, 0, len(targets))}
	for _, target := range targets {
		outcome := channelModelResetOutcome{UUID: target.UUID, Name: target.Name}
		if ctx.Err() != nil {
			summary.Failed++
			outcome.Message = "The request was canceled before this channel was reset."
			summary.Results = append(summary.Results, outcome)
			continue
		}
		channel, err := model.ResetChannelModelsToDefaults(ctx, target.Id, defaultChannelResetCatalog)
		if err != nil {
			var conflict *model.ChannelModelResetConflict
			if errors.As(err, &conflict) {
				summary.Rejected++
				outcome.Conflict = conflict
				outcome.Message = conflict.Error()
			} else {
				summary.Failed++
				outcome.Message = "Failed to reset this channel. No changes were applied to it."
				if lg != nil {
					lg.Error("bulk channel model reset failed", zap.String("channel_uuid", target.UUID), zap.Error(err))
				}
			}
		} else {
			summary.Reset++
			outcome.Success = true
			outcome.ModelCount = len(channel.GetSupportedModelNames())
		}
		summary.Results = append(summary.Results, outcome)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": summary})
}
