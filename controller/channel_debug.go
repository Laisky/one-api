package controller

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/idresolve"
	"github.com/Laisky/one-api/model"
)

// DebugChannelModelConfigs logs model-config diagnostics for one channel.
// Parameters:
//   - c: Gin context carrying the channel UUID in the :id path parameter.
//
// Return values: none.
//
// A malformed reference answers HTTP 400; an unknown channel or a lookup
// failure follows the shared respondResourceRefError contract.
func DebugChannelModelConfigs(c *gin.Context) {
	channelId, err := resolveChannelRef(c.Param("id"))
	if err != nil {
		respondResourceRefError(c, err)
		return
	}

	err = model.DebugChannelModelConfigs(channelId)
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.Wrap(err, "Debug failed"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Debug information logged. Check application logs for details.",
	})
}

// DebugAllChannelModelConfigs provides summary debugging information for all channels
func DebugAllChannelModelConfigs(c *gin.Context) {
	err := model.DebugAllChannelModelConfigs()
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.Wrap(err, "Debug failed"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Debug summary logged. Check application logs for details.",
	})
}

// FixChannelModelConfigs attempts to repair one channel's model configuration.
// Parameters:
//   - c: Gin context carrying the channel UUID in the :id path parameter.
//
// Return values: none.
//
// A malformed reference answers HTTP 400; an unknown channel or a lookup
// failure follows the shared respondResourceRefError contract.
func FixChannelModelConfigs(c *gin.Context) {
	channelId, err := resolveChannelRef(c.Param("id"))
	if err != nil {
		respondResourceRefError(c, err)
		return
	}

	err = model.FixChannelModelConfigs(channelId)
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.Wrap(err, "Fix failed"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Channel model configs fixed. Check application logs for details.",
	})
}

// ValidateAllChannelModelConfigs validates all channels and reports issues
func ValidateAllChannelModelConfigs(c *gin.Context) {
	err := model.ValidateAllChannelModelConfigs()
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.Wrap(err, "Validation failed"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Validation completed. Check application logs for details.",
	})
}

// RemigratAllChannels re-runs migration for all channels
func RemigratAllChannels(c *gin.Context) {
	err := model.MigrateAllChannelModelConfigs()
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.Wrap(err, "Re-migration failed"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Re-migration completed. Check application logs for details.",
	})
}

// GetChannelMigrationStatus returns the migration status of a specific channel.
// Parameters:
//   - c: Gin context containing the channel UUID path parameter and response writer.
//
// Return values: none.
//
// A malformed reference answers HTTP 400. An unknown channel, including one
// deleted between resolution and the row load, is a not-found reference, and
// any other lookup failure is a server error; both follow the shared
// respondResourceRefError contract.
func GetChannelMigrationStatus(c *gin.Context) {
	channelId, err := resolveChannelRef(c.Param("id"))
	if err != nil {
		respondResourceRefError(c, err)
		return
	}

	var channel model.Channel
	err = model.DB.Where("id = ?", channelId).First(&channel).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondResourceRefError(c, errors.WithStack(idresolve.ErrNotFound))
			return
		}
		respondResourceRefError(c, errors.Wrap(err, "load channel for migration status"))
		return
	}

	status := map[string]any{
		"channel_uuid":         channel.UUID,
		"channel_name":         channel.Name,
		"channel_type":         channel.Type,
		"has_model_configs":    channel.ModelConfigs != nil && *channel.ModelConfigs != "" && *channel.ModelConfigs != "{}",
		"has_model_ratio":      channel.ModelRatio != nil && *channel.ModelRatio != "" && *channel.ModelRatio != "{}",
		"has_completion_ratio": channel.CompletionRatio != nil && *channel.CompletionRatio != "" && *channel.CompletionRatio != "{}",
	}

	// Get model counts
	if status["has_model_configs"].(bool) {
		configs := channel.GetModelPriceConfigs()
		if configs != nil {
			var modelNames []string
			for modelName := range configs {
				modelNames = append(modelNames, modelName)
			}
			status["model_configs_models"] = modelNames
			status["model_configs_count"] = len(modelNames)
		}
	}

	if status["has_model_ratio"].(bool) {
		ratios := channel.GetModelRatio()
		if ratios != nil {
			var modelNames []string
			for modelName := range ratios {
				modelNames = append(modelNames, modelName)
			}
			status["model_ratio_models"] = modelNames
			status["model_ratio_count"] = len(modelNames)
		}
	}

	// Determine migration status
	migrationStatus := "unknown"
	if status["has_model_configs"].(bool) {
		if status["has_model_ratio"].(bool) || status["has_completion_ratio"].(bool) {
			migrationStatus = "migrated_with_legacy"
		} else {
			migrationStatus = "migrated"
		}
	} else if status["has_model_ratio"].(bool) || status["has_completion_ratio"].(bool) {
		migrationStatus = "needs_migration"
	} else {
		migrationStatus = "empty"
	}

	status["migration_status"] = migrationStatus

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

// CleanAllMixedModelData cleans all channels that have mixed model data
func CleanAllMixedModelData(c *gin.Context) {
	err := model.CleanAllMixedModelData()
	if err != nil {
		helper.RespondErrorWithStatus(c, http.StatusInternalServerError, errors.Wrap(err, "Cleaning failed"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mixed model data cleaned. Check application logs for details.",
	})
}
