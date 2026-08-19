package model

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/channeltype"
)

// MigrateCustomChannelsToOpenAICompatible upgrades legacy custom channels (type 8) to the unified
// OpenAI-compatible channel type. The migration is idempotent and safe to run on every startup.
// It ensures old deployments seamlessly adopt the enhanced OpenAI-compatible behaviour without
// requiring manual intervention from operators.
func MigrateCustomChannelsToOpenAICompatible() error {
	if DB == nil {
		return errors.New("database not initialized")
	}

	if !DB.Migrator().HasTable(&Channel{}) {
		logger.Logger.Debug("channels table missing, skipping custom channel migration")
		return nil
	}

	var legacyCount int64
	if err := DB.Model(&Channel{}).
		Where("type = ?", channeltype.Custom).
		Count(&legacyCount).Error; err != nil {
		return errors.Wrap(err, "count legacy custom channels")
	}
	if legacyCount == 0 {
		logger.Logger.Debug("no legacy custom channels detected")
		return nil
	}

	logger.Logger.Info("migrating legacy custom channels to openai-compatible",
		zap.Int64("legacy_count", legacyCount))

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Channel{}).
			Where("type = ?", channeltype.Custom).
			Update("type", channeltype.OpenAICompatible).Error; err != nil {
			return errors.Wrap(err, "update channel type to openai compatible")
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "migrate custom channel type to openai compatible")
	}

	logger.Logger.Info("custom channel migration completed",
		zap.Int64("migrated_count", legacyCount))
	return nil
}
