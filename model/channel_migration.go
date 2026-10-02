package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	mysql_driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
)

// Migration control & state
var (
	channelFieldMigrationOnce sync.Once
	channelFieldMigrated      atomic.Bool // true after successful schema migration
)

// isMySQLDataTooLongErr checks whether an error is a MySQL "data too long" error (code 1406)
func isMySQLDataTooLongErr(err error) bool {
	if err == nil {
		return false
	}
	var merr *mysql_driver.MySQLError
	if errors.As(err, &merr) {
		if merr.Number == 1406 { // ER_DATA_TOO_LONG
			return true
		}
	}
	// fallback substring match (defensive for wrapped errors)
	if strings.Contains(err.Error(), "Data too long for column") {
		return true
	}
	return false
}

// MigrateModelConfigsToModelPrice migrates existing ModelConfigs data from the old format
// (map[string]ModelConfig) to the new format (map[string]ModelPriceLocal)
// This handles cases where contributors have already applied the PR changes locally
func (channel *Channel) MigrateModelConfigsToModelPrice() error {
	if channel.ModelConfigs == nil || *channel.ModelConfigs == "" || *channel.ModelConfigs == "{}" {
		return nil // Nothing to migrate
	}

	// Validate JSON format first
	var rawData any
	if err := json.Unmarshal([]byte(*channel.ModelConfigs), &rawData); err != nil {
		return identity.Tag(
			errors.Wrapf(err, "invalid JSON in ModelConfigs for channel %d", channel.Id),
			channel.Ref())
	}

	// Check if the JSON is null, array, or string (invalid types)
	switch rawData.(type) {
	case nil:
		return identity.Tag(
			errors.Errorf("ModelConfigs cannot be parsed: null value for channel %d", channel.Id),
			channel.Ref())
	case []any:
		return identity.Tag(
			errors.Errorf("ModelConfigs cannot be parsed: array value for channel %d", channel.Id),
			channel.Ref())
	case string:
		return identity.Tag(
			errors.Errorf("ModelConfigs cannot be parsed: string value for channel %d", channel.Id),
			channel.Ref())
	}

	// Try to unmarshal as the new format first
	var newFormatConfigs map[string]ModelConfigLocal
	err := json.Unmarshal([]byte(*channel.ModelConfigs), &newFormatConfigs)
	if err == nil {
		// Validate the new format data
		if err := channel.validateModelPriceConfigs(newFormatConfigs); err != nil {
			return identity.Tag(
				errors.Wrapf(err, "invalid ModelPriceLocal data for channel %d", channel.Id),
				channel.Ref())
		}

		// Check if it has pricing data (already in new format)
		hasPricingData := false
		for _, config := range newFormatConfigs {
			if config.Ratio != 0 || config.CompletionRatio != 0 {
				hasPricingData = true
				break
			}
		}

		if hasPricingData {
			logger.Logger.Info("Channel ModelConfigs already in new format with pricing data", channel.Ref().Zap()...)
			return nil
		}

		logger.Logger.Info("Channel ModelConfigs in new format but needs pricing migration", channel.Ref().Zap()...)
	}

	// Try to unmarshal as the old format (map[string]ModelConfig)
	var oldFormatConfigs map[string]ModelConfig
	err = json.Unmarshal([]byte(*channel.ModelConfigs), &oldFormatConfigs)
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "ModelConfigs cannot be parsed in either format for channel %d", channel.Id),
			channel.Ref())
	}

	// Validate old format data
	for modelName, config := range oldFormatConfigs {
		if modelName == "" {
			return identity.Tag(
				errors.Errorf("empty model name found in ModelConfigs for channel %d", channel.Id),
				channel.Ref())
		}
		if config.MaxTokens < 0 {
			return identity.Tag(
				errors.Errorf("negative MaxTokens for model %s in channel %d", modelName, channel.Id),
				channel.Ref())
		}
	}

	// Convert old format to new format
	migratedConfigs := make(map[string]ModelConfigLocal)

	// Get existing ModelRatio and CompletionRatio for this channel
	modelRatios := channel.GetModelRatio()
	completionRatios := channel.GetCompletionRatio()

	// Collect all model names from all sources
	allModelNames := make(map[string]bool)
	for modelName := range oldFormatConfigs {
		if modelName != "" {
			allModelNames[modelName] = true
		}
	}
	for modelName := range modelRatios {
		if modelName != "" {
			allModelNames[modelName] = true
		}
	}
	for modelName := range completionRatios {
		if modelName != "" {
			allModelNames[modelName] = true
		}
	}

	// Process all models from all sources
	for modelName := range allModelNames {
		newConfig := ModelConfigLocal{}

		// Start with MaxTokens from old config if available
		if oldConfig, exists := oldFormatConfigs[modelName]; exists {
			newConfig.MaxTokens = oldConfig.MaxTokens
			if oldConfig.Image != nil {
				normalizedImage, err := normalizeImagePricingLocal(oldConfig.Image)
				if err != nil {
					return errors.Wrapf(err, "invalid legacy image pricing for model %s", modelName)
				}
				newConfig.Image = normalizedImage
			} else if oldConfig.ImagePriceUsd > 0 {
				newConfig.Image = &ImagePricingLocal{PricePerImageUsd: oldConfig.ImagePriceUsd}
			}
		}

		// Add pricing information if available
		if modelRatios != nil {
			if ratio, exists := modelRatios[modelName]; exists {
				if ratio < 0 {
					return errors.Errorf("negative ratio for model %s: %f", modelName, ratio)
				}
				if ratio > 0 {
					newConfig.Ratio = ratio
				}
			}
		}
		if completionRatios != nil {
			if completionRatio, exists := completionRatios[modelName]; exists {
				if completionRatio < 0 {
					return errors.Errorf("negative completion ratio for model %s: %f", modelName, completionRatio)
				}
				if completionRatio > 0 {
					newConfig.CompletionRatio = completionRatio
				}
			}
		}

		migratedConfigs[modelName] = newConfig
	}

	// Validate migrated data
	if err := channel.validateModelPriceConfigs(migratedConfigs); err != nil {
		return identity.Tag(
			errors.Wrapf(err, "migration produced invalid data for channel %d", channel.Id),
			channel.Ref())
	}

	// Save the migrated data back to ModelConfigs
	jsonBytes, err := json.Marshal(migratedConfigs)
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to marshal migrated data for channel %d", channel.Id),
			channel.Ref())
	}

	jsonStr := string(jsonBytes)
	channel.ModelConfigs = &jsonStr

	logger.Logger.Info("Successfully migrated ModelConfigs from old format to new format",
		append(channel.Ref().Zap(), zap.Int("model_count", len(migratedConfigs)))...)
	return nil
}

func migrateLegacyImagePriceInConfigs(rawJSON string) (string, bool, error) {
	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" || trimmed == "{}" {
		return rawJSON, false, nil
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return "", false, errors.Wrap(err, "decode legacy image pricing map")
	}
	changed := false
	for modelName, rawCfg := range payload {
		cfgMap, ok := rawCfg.(map[string]any)
		if !ok || cfgMap == nil {
			continue
		}
		legacyValue, hasLegacy := cfgMap["image_price_usd"]
		if !hasLegacy {
			continue
		}
		price, ok := floatFromAny(legacyValue)
		delete(cfgMap, "image_price_usd")
		if !ok || price <= 0 {
			payload[modelName] = cfgMap
			changed = true
			continue
		}
		imageValue, hasImage := cfgMap["image"]
		var imageMap map[string]any
		if hasImage {
			imageMap, _ = imageValue.(map[string]any)
		}
		if imageMap == nil {
			imageMap = make(map[string]any)
		}
		if existing, exists := imageMap["price_per_image_usd"]; !exists {
			imageMap["price_per_image_usd"] = price
		} else if existingPrice, ok := floatFromAny(existing); ok && existingPrice <= 0 {
			imageMap["price_per_image_usd"] = price
		}
		cfgMap["image"] = imageMap
		payload[modelName] = cfgMap
		changed = true
	}
	if !changed {
		return rawJSON, false, nil
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return "", false, errors.Wrap(err, "encode normalized image pricing")
	}
	return string(normalized), true, nil
}

// MigrateHistoricalPricingToModelConfigs migrates historical ModelRatio and CompletionRatio data
// into the new unified ModelConfigs format for a single channel
func (channel *Channel) MigrateHistoricalPricingToModelConfigs() error {
	// Validate channel
	if channel == nil {
		return errors.New("channel is nil")
	}

	// Get existing ModelRatio and CompletionRatio data with validation
	var modelRatios map[string]float64
	var completionRatios map[string]float64
	var migrationErrors []string

	// Safely get ModelRatio
	if channel.ModelRatio != nil && *channel.ModelRatio != "" && *channel.ModelRatio != "{}" {
		if err := json.Unmarshal([]byte(*channel.ModelRatio), &modelRatios); err != nil {
			migrationErrors = append(migrationErrors, fmt.Sprintf("invalid ModelRatio JSON: %s", err.Error()))
		} else {
			// Validate ModelRatio values
			for modelName, ratio := range modelRatios {
				if modelName == "" {
					migrationErrors = append(migrationErrors, "empty model name in ModelRatio")
				}
				if ratio < 0 {
					migrationErrors = append(migrationErrors, fmt.Sprintf("negative ratio for model %s: %f", modelName, ratio))
				}
			}
		}
	}

	// Safely get CompletionRatio
	if channel.CompletionRatio != nil && *channel.CompletionRatio != "" && *channel.CompletionRatio != "{}" {
		if err := json.Unmarshal([]byte(*channel.CompletionRatio), &completionRatios); err != nil {
			migrationErrors = append(migrationErrors, fmt.Sprintf("invalid CompletionRatio JSON: %s", err.Error()))
		} else {
			// Validate CompletionRatio values
			for modelName, ratio := range completionRatios {
				if modelName == "" {
					migrationErrors = append(migrationErrors, "empty model name in CompletionRatio")
				}
				if ratio < 0 {
					migrationErrors = append(migrationErrors, fmt.Sprintf("negative completion ratio for model %s: %f", modelName, ratio))
				}
			}
		}
	}

	// Report validation errors but continue with valid data
	if len(migrationErrors) > 0 {
		logger.Logger.Error("Channel has validation errors in historical data",
			append(channel.Ref().Zap(), zap.Any("errors", migrationErrors))...)
		// Don't return error - continue with valid data
	}

	// Skip if no valid historical data to migrate
	if len(modelRatios) == 0 && len(completionRatios) == 0 {
		return nil
	}

	// Check if ModelConfigs already has unified data
	existingConfigs := channel.GetModelPriceConfigs()
	if len(existingConfigs) > 0 {
		// Check if existing configs have pricing data (not just MaxTokens)
		hasPricingData := false
		for _, config := range existingConfigs {
			if config.Ratio != 0 || config.CompletionRatio != 0 {
				hasPricingData = true
				break
			}
		}

		if hasPricingData {
			logger.Logger.Info("Channel already has pricing data in ModelConfigs, skipping historical migration",
				channel.Ref().Zap()...)
			return nil
		}

		// Merge historical pricing with existing MaxTokens data
		logger.Logger.Info("Channel has MaxTokens data, merging with historical pricing", channel.Ref().Zap()...)
	} else {
		existingConfigs = make(map[string]ModelConfigLocal)
	}

	// Collect all valid model names from both ratios and existing configs
	allModelNames := make(map[string]bool)
	for modelName, ratio := range modelRatios {
		// Skip invalid entries
		if modelName != "" && ratio >= 0 {
			allModelNames[modelName] = true
		}
	}
	for modelName, ratio := range completionRatios {
		// Skip invalid entries
		if modelName != "" && ratio >= 0 {
			allModelNames[modelName] = true
		}
	}
	for modelName := range existingConfigs {
		if modelName != "" {
			allModelNames[modelName] = true
		}
	}

	// Create unified ModelConfigs from all data sources
	modelConfigs := make(map[string]ModelConfigLocal)
	for modelName := range allModelNames {
		config := ModelConfigLocal{}

		// Start with existing config if available
		if existingConfig, exists := existingConfigs[modelName]; exists {
			config = existingConfig
		}

		// Add/override pricing data from historical sources (only valid data)
		if modelRatios != nil {
			if ratio, exists := modelRatios[modelName]; exists && ratio >= 0 {
				config.Ratio = ratio
			}
		}

		if completionRatios != nil {
			if completionRatio, exists := completionRatios[modelName]; exists && completionRatio >= 0 {
				config.CompletionRatio = completionRatio
			}
		}

		// Add if we have any data (pricing or MaxTokens)
		if config.Ratio != 0 || config.CompletionRatio != 0 || config.MaxTokens != 0 {
			modelConfigs[modelName] = config
		}
	}

	// Save the migrated data to ModelConfigs
	if len(modelConfigs) > 0 {
		// Log the models being migrated for debugging
		var modelNames []string
		for modelName := range modelConfigs {
			modelNames = append(modelNames, modelName)
		}
		logger.Logger.Info("Channel migrating models",
			append(channel.Ref().Zap(), zap.Int("type", channel.Type), zap.Strings("models", modelNames))...)

		err := channel.SetModelPriceConfigs(modelConfigs)
		if err != nil {
			logger.Logger.Error("Failed to set migrated ModelConfigs for channel",
				append(channel.Ref().Zap(), zap.Error(err))...)
			return identity.Tag(
				errors.Wrapf(err, "set migrated model configs for channel %d", channel.Id),
				channel.Ref())
		}

		logger.Logger.Info("Successfully migrated historical pricing data to ModelConfigs",
			append(channel.Ref().Zap(), zap.Int("model_count", len(modelConfigs)))...)
	}

	return nil
}

// MigrateChannelFieldsToText migrates ModelConfigs and ModelMapping fields from varchar(1024) to text type.
//
// Background:
// The original varchar(1024) length was insufficient for complex model configurations, especially when:
// - Multiple models are configured with detailed pricing information (ratio, completion_ratio, max_tokens)
// - Long model names or complex mapping values are used
// - Channel-specific configurations grow beyond the 1024 character limit
//
// This migration is essential because:
// 1. Modern AI models have longer names and more complex configurations
// 2. Users need to configure pricing for dozens of models per channel
// 3. JSON serialization of comprehensive model configs easily exceeds 1024 chars
// 4. Truncated configurations lead to data loss and system errors
//
// The migration is designed to be:
// - Idempotent: Can be run multiple times safely
// - Database-agnostic: Supports MySQL, PostgreSQL, and SQLite
// - Data-preserving: All existing data is maintained during the migration
// - Transaction-safe: Uses database transactions to ensure data integrity
//
// This function should be called during application startup before any channel operations.
func MigrateChannelFieldsToText() error {
	// Ensure only executed once even if called from multiple goroutines
	var runErr error
	channelFieldMigrationOnce.Do(func() {
		logger.Logger.Info("Starting migration of ModelConfigs and ModelMapping fields to TEXT type")

		// Skip if we already migrated in this process
		if channelFieldMigrated.Load() {
			logger.Logger.Info("Channel field migration already completed in this process - skipping")
			return
		}

		needsMigration, err := checkIfFieldMigrationNeeded()
		if err != nil {
			runErr = errors.Wrap(err, "failed to check migration status")
			return
		}

		if !needsMigration {
			logger.Logger.Info("ModelConfigs and ModelMapping fields are already TEXT type - no migration needed")
			channelFieldMigrated.Store(true)
			return
		}

		logger.Logger.Info("Column type migration required - proceeding with migration")
		runErr = performFieldMigration()
	})
	return runErr
}

// performFieldMigration executes the actual database schema changes to migrate fields to TEXT type.
// This function uses database transactions to ensure data integrity and provides detailed error handling.
func performFieldMigration() error {
	// Use transaction for data integrity - ensures all-or-nothing migration
	tx := DB.Begin()
	if tx.Error != nil {
		return errors.Wrap(tx.Error, "failed to start transaction")
	}

	// Ensure transaction is properly handled in case of panic or error
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			logger.Logger.Error("Column migration panicked, rolled back",
				zap.Any("panic", r))
		}
	}()

	// Perform database-specific column type changes
	var err error
	if common.UsingMySQL.Load() {
		err = performMySQLFieldMigration(tx)
	} else if common.UsingPostgreSQL.Load() {
		err = performPostgreSQLFieldMigration(tx)
	} else {
		// This should not happen due to the check in checkIfFieldMigrationNeeded,
		// but we handle it for safety
		tx.Rollback()
		return errors.New("unsupported database type for field migration")
	}

	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "perform field migration")
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return errors.Wrap(err, "failed to commit migration")
	}

	logger.Logger.Info("Successfully migrated ModelConfigs and ModelMapping columns to TEXT type")
	return nil
}

// performMySQLFieldMigration performs the MySQL-specific column type migration.
func performMySQLFieldMigration(tx *gorm.DB) error {
	logger.Logger.Info("Performing MySQL field migration")

	// MySQL: Use MODIFY COLUMN to change type while preserving data.
	// Do NOT set DEFAULT '' on TEXT columns (not allowed for TEXT/BLOB in MySQL).
	err := tx.Exec("ALTER TABLE channels MODIFY COLUMN model_configs TEXT").Error
	if err != nil {
		return errors.Wrap(err, "failed to migrate model_configs column")
	}

	err = tx.Exec("ALTER TABLE channels MODIFY COLUMN model_mapping TEXT").Error
	if err != nil {
		return errors.Wrap(err, "failed to migrate model_mapping column")
	}

	channelFieldMigrated.Store(true)
	logger.Logger.Info("MySQL field migration completed successfully")
	return nil
}

// performPostgreSQLFieldMigration performs the PostgreSQL-specific column type migration.
func performPostgreSQLFieldMigration(tx *gorm.DB) error {
	logger.Logger.Info("Performing PostgreSQL field migration")

	// PostgreSQL: Use ALTER COLUMN TYPE to change column type
	err := tx.Exec("ALTER TABLE channels ALTER COLUMN model_configs TYPE TEXT").Error
	if err != nil {
		return errors.Wrap(err, "failed to migrate model_configs column")
	}

	err = tx.Exec("ALTER TABLE channels ALTER COLUMN model_mapping TYPE TEXT").Error
	if err != nil {
		return errors.Wrap(err, "failed to migrate model_mapping column")
	}

	channelFieldMigrated.Store(true)
	logger.Logger.Info("PostgreSQL field migration completed successfully")
	return nil
}

// checkIfFieldMigrationNeeded checks if ModelConfigs and ModelMapping fields need to be migrated to TEXT type.
// This function provides idempotency by checking the current column types in the database.
// Returns true if migration is needed, false if fields are already TEXT type.
func checkIfFieldMigrationNeeded() (bool, error) {
	if common.UsingMySQL.Load() {
		// First check if the channels table exists at all
		var tableExists int
		err := DB.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'channels'`).
			Scan(&tableExists).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check if channels table exists in MySQL")
		}

		// If table doesn't exist, no migration needed - AutoMigrate will create it correctly
		if tableExists == 0 {
			logger.Logger.Info("Channels table does not exist - no field migration needed")
			return false, nil
		}

		// Check MySQL column types for both fields
		var modelConfigsType, modelMappingType string

		// Check model_configs column type
		err = DB.Raw(`SELECT DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'channels' AND COLUMN_NAME = 'model_configs'`).
			Scan(&modelConfigsType).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check model_configs column type in MySQL")
		}

		// Check model_mapping column type
		err = DB.Raw(`SELECT DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'channels' AND COLUMN_NAME = 'model_mapping'`).
			Scan(&modelMappingType).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check model_mapping column type in MySQL")
		}

		logger.Logger.Info("Detected MySQL column types for migration check",
			zap.String("model_configs_type", modelConfigsType),
			zap.String("model_mapping_type", modelMappingType))

		// If columns don't exist, no migration needed - AutoMigrate will create them correctly
		if modelConfigsType == "" || modelMappingType == "" {
			logger.Logger.Info("One or more columns do not exist - no field migration needed")
			return false, nil
		}

		// Migration needed unless both columns already some kind of TEXT.* (varchar is insufficient)
		isTextType := func(tp string) bool { return strings.Contains(tp, "text") }
		need := !(isTextType(modelConfigsType) && isTextType(modelMappingType))
		return need, nil

	} else if common.UsingPostgreSQL.Load() {
		// First check if the channels table exists at all
		var tableExists int
		err := DB.Raw(`SELECT COUNT(*) FROM information_schema.tables
			WHERE table_name = 'channels'`).
			Scan(&tableExists).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check if channels table exists in PostgreSQL")
		}

		// If table doesn't exist, no migration needed - AutoMigrate will create it correctly
		if tableExists == 0 {
			logger.Logger.Info("Channels table does not exist - no field migration needed")
			return false, nil
		}

		// Check PostgreSQL column types for both fields
		var modelConfigsType, modelMappingType string

		// Check model_configs column type
		err = DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_name = 'channels' AND column_name = 'model_configs'`).
			Scan(&modelConfigsType).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check model_configs column type in PostgreSQL")
		}

		// Check model_mapping column type
		err = DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_name = 'channels' AND column_name = 'model_mapping'`).
			Scan(&modelMappingType).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check model_mapping column type in PostgreSQL")
		}

		// If columns don't exist, no migration needed - AutoMigrate will create them correctly
		if modelConfigsType == "" || modelMappingType == "" {
			logger.Logger.Info("One or more columns do not exist - no field migration needed")
			return false, nil
		}

		// Migration needed if either field is still character varying (varchar)
		return modelConfigsType == "character varying" || modelMappingType == "character varying", nil

	} else if common.UsingSQLite.Load() {
		// SQLite is flexible with column types and doesn't enforce strict typing
		// TEXT and VARCHAR are treated the same way, so no migration is needed
		logger.Logger.Info("SQLite detected - column type migration not required (SQLite is flexible with text types)")
		return false, nil

	} else {
		// Unknown database type - assume no migration needed to be safe
		logger.Logger.Info("Unknown database type detected - skipping column type migration")
		return false, nil
	}
}

// MigrateAllChannelModelConfigs migrates all channels' ModelConfigs from old format to new format
// and also migrates historical ModelRatio/CompletionRatio data to the new unified format
// This should be called during application startup to handle existing data
func MigrateAllChannelModelConfigs() error {
	logger.Logger.Info("Starting migration of all channel ModelConfigs and historical pricing data")

	var channels []*Channel
	err := DB.Find(&channels).Error
	if err != nil {
		return errors.Wrap(err, "failed to fetch channels")
	}

	if len(channels) == 0 {
		logger.Logger.Info("No channels found for migration")
		return nil
	}

	migratedCount := 0
	historicalMigratedCount := 0
	errorCount := 0
	var migrationErrors []string

	for _, channel := range channels {
		channelUpdated := false
		originalModelConfigs := ""
		if channel.ModelConfigs != nil {
			originalModelConfigs = *channel.ModelConfigs
		}

		// First, migrate existing ModelConfigs from old format to new format (PR format -> unified format)
		if channel.ModelConfigs != nil && *channel.ModelConfigs != "" && *channel.ModelConfigs != "{}" {
			err := channel.MigrateModelConfigsToModelPrice()
			if err != nil {
				logger.Logger.Error("Failed to migrate ModelConfigs for channel", append(channel.Ref().Zap(), zap.Error(err))...)
				errorMsg := getMigrationErrorContext(err, channel.Id, "ModelConfigs format migration")
				migrationErrors = append(migrationErrors, errorMsg)
				errorCount++
				continue
			}
			channelUpdated = true
			migratedCount++
		}

		// Second, migrate historical ModelRatio/CompletionRatio data to ModelConfigs
		err := channel.MigrateHistoricalPricingToModelConfigs()
		if err != nil {
			logger.Logger.Error("Failed to migrate historical pricing for channel",
				append(channel.Ref().Zap(), zap.Error(err))...)
			errorMsg := getMigrationErrorContext(err, channel.Id, "historical pricing migration")
			migrationErrors = append(migrationErrors, errorMsg)
			errorCount++
			continue
		}

		// Check if historical migration actually created ModelConfigs data
		if channel.ModelConfigs != nil && *channel.ModelConfigs != "" && *channel.ModelConfigs != "{}" {
			if !channelUpdated { // Only count if it wasn't already counted in the first migration
				historicalMigratedCount++
				channelUpdated = true
			}
		}

		// Save the migrated channel back to database if any changes were made
		if channelUpdated {
			// Validate the final result before saving
			finalConfigs := channel.GetModelPriceConfigs()
			if err := channel.validateModelPriceConfigs(finalConfigs); err != nil {
				logger.Logger.Error("Migration validation failed for channel", append(channel.Ref().Zap(), zap.Error(err))...)
				errorMsg := getMigrationErrorContext(err, channel.Id, "validation")
				migrationErrors = append(migrationErrors, errorMsg)
				errorCount++
				// Restore original data
				if originalModelConfigs != "" {
					channel.ModelConfigs = &originalModelConfigs
				} else {
					channel.ModelConfigs = nil
				}
				continue
			}

			saveErr := DB.Model(channel).Update("model_configs", channel.ModelConfigs).Error
			if saveErr != nil {
				// Detect MySQL column size overflow and attempt on-the-fly migration+retry
				if common.UsingMySQL.Load() && isMySQLDataTooLongErr(saveErr) {
					logger.Logger.Warn("Detected model_configs length overflow, attempting column type migration to TEXT and retry",
						channel.Ref().Zap()...)
					if migErr := performMySQLFieldMigration(DB); migErr != nil {
						logger.Logger.Error("On-demand MySQL column migration failed", append(channel.Ref().Zap(), zap.Error(migErr))...)
						errorMsg := fmt.Sprintf("Failed to save migrated ModelConfigs for channel %d after overflow & migration attempt: %s", channel.Id, saveErr.Error())
						migrationErrors = append(migrationErrors, errorMsg)
						errorCount++
						continue
					}
					// Retry save after migration
					if retryErr := DB.Model(channel).Update("model_configs", channel.ModelConfigs).Error; retryErr != nil {
						logger.Logger.Error("Retry save after column migration still failed",
							append(channel.Ref().Zap(), zap.Error(retryErr))...)
						errorMsg := fmt.Sprintf("Failed to save migrated ModelConfigs for channel %d after retry: %s", channel.Id, retryErr.Error())
						migrationErrors = append(migrationErrors, errorMsg)
						errorCount++
						continue
					}
					logger.Logger.Info("Retry save after on-demand column migration succeeded", channel.Ref().Zap()...)
				} else {
					logger.Logger.Error("Failed to save migrated ModelConfigs for channel",
						append(channel.Ref().Zap(), zap.Error(saveErr))...)
					errorMsg := fmt.Sprintf("Failed to save migrated ModelConfigs for channel %d: %s", channel.Id, saveErr.Error())
					migrationErrors = append(migrationErrors, errorMsg)
					errorCount++
					continue
				}
			}
		}
	}

	// If more than 50% of channels failed, return error to prevent silent data loss
	if len(channels) > 0 {
		failureRate := float64(errorCount) / float64(len(channels))
		if failureRate > 0.5 {
			return errors.Errorf("migration failed for %d/%d channels (%.1f%%)",
				errorCount, len(channels), failureRate*100)
		}
	}

	// Log final results
	if migratedCount > 0 {
		logger.Logger.Info("Successfully migrated ModelConfigs format", zap.Int("migrated_count", migratedCount))
	}
	if historicalMigratedCount > 0 {
		logger.Logger.Info("Successfully migrated historical pricing data", zap.Int("historical_migrated_count", historicalMigratedCount))
	}
	if errorCount > 0 {
		logger.Logger.Error("Migration completed with errors", zap.Int("error_count", errorCount))
		for _, errMsg := range migrationErrors {
			logger.Logger.Error("Migration error", zap.String("error", errMsg))
		}
	}
	if migratedCount == 0 && historicalMigratedCount == 0 && errorCount == 0 {
		logger.Logger.Info("No channels required data migration")
	}

	return nil
}

// MigrateChannelLegacyImagePricing normalizes legacy per-image pricing fields into Image configs.
func MigrateChannelLegacyImagePricing() error {
	logger.Logger.Info("Starting migration of legacy channel image pricing")
	var channels []*Channel
	if err := DB.Find(&channels).Error; err != nil {
		return errors.Wrap(err, "fetch channels for image pricing migration")
	}
	migrated := 0
	for _, channel := range channels {
		if channel.ModelConfigs == nil || *channel.ModelConfigs == "" || *channel.ModelConfigs == "{}" {
			continue
		}
		updated, changed, err := migrateLegacyImagePriceInConfigs(*channel.ModelConfigs)
		if err != nil {
			logger.Logger.Error("failed to normalize image pricing for channel", append(channel.Ref().Zap(), zap.Error(err))...)
			continue
		}
		if !changed {
			continue
		}
		original := *channel.ModelConfigs
		channel.ModelConfigs = &updated
		configs := channel.GetModelPriceConfigs()
		if err := channel.validateModelPriceConfigs(configs); err != nil {
			logger.Logger.Error("validated migrated image pricing failed", append(channel.Ref().Zap(), zap.Error(err))...)
			channel.ModelConfigs = &original
			continue
		}
		if err := DB.Model(channel).Update("model_configs", channel.ModelConfigs).Error; err != nil {
			logger.Logger.Error("failed to persist migrated image pricing", append(channel.Ref().Zap(), zap.Error(err))...)
			channel.ModelConfigs = &original
			continue
		}
		migrated++
	}
	if migrated > 0 {
		logger.Logger.Info("Legacy channel image pricing migration completed", zap.Int("channels_migrated", migrated))
	} else {
		logger.Logger.Info("No legacy channel image pricing entries required migration")
	}
	return nil
}

// getMigrationErrorContext provides additional context for migration errors
func getMigrationErrorContext(err error, channelID int, operation string) string {
	if err == nil {
		return ""
	}

	context := fmt.Sprintf("Channel %d %s failed", channelID, operation)

	// Add specific guidance for common errors
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "Data too long"):
		context += " - Column size insufficient, consider running field migration"
	case strings.Contains(errStr, "invalid character"):
		context += " - Invalid JSON format in configuration data"
	case strings.Contains(errStr, "connection"):
		context += " - Database connection issue, check connectivity"
	case strings.Contains(errStr, "syntax error"):
		context += " - SQL syntax error, check database compatibility"
	case strings.Contains(errStr, "duplicate"):
		context += " - Duplicate key constraint violation"
	default:
		context += fmt.Sprintf(" - %s", err.Error())
	}

	return context
}
