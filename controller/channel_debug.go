package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/model"
)

// DebugChannelModelConfigs provides debugging information for a specific channel
func DebugChannelModelConfigs(c *gin.Context) {
	channelIdStr := c.Param("id")
	channelId, err := strconv.Atoi(channelIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid channel ID",
		})
		return
	}

	err = model.DebugChannelModelConfigs(channelId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Debug failed: " + err.Error(),
		})
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Debug failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Debug summary logged. Check application logs for details.",
	})
}

// FixChannelModelConfigs attempts to fix a specific channel's model configuration
func FixChannelModelConfigs(c *gin.Context) {
	channelIdStr := c.Param("id")
	channelId, err := strconv.Atoi(channelIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid channel ID",
		})
		return
	}

	err = model.FixChannelModelConfigs(channelId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Fix failed: " + err.Error(),
		})
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Validation failed: " + err.Error(),
		})
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Re-migration failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Re-migration completed. Check application logs for details.",
	})
}

// GetChannelMigrationStatus returns the migration status of a specific channel
func GetChannelMigrationStatus(c *gin.Context) {
	channelIdStr := c.Param("id")
	channelId, err := strconv.Atoi(channelIdStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid channel ID",
		})
		return
	}

	var channel model.Channel
	err = model.DB.Where("id = ?", channelId).First(&channel).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Channel not found",
		})
		return
	}

	status := map[string]any{
		"channel_id":           channel.Id,
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
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Cleaning failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mixed model data cleaned. Check application logs for details.",
	})
}
