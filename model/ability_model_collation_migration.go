package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/logger"
)

var mysqlAbilityModelColumnTypePattern = regexp.MustCompile(`(?i)^(var)?char\(([1-9][0-9]{0,4})\)$`)

type mysqlAbilityModelColumn struct {
	ColumnType   string  `gorm:"column:column_type"`
	CharacterSet string  `gorm:"column:character_set_name"`
	Collation    string  `gorm:"column:collation_name"`
	IsNullable   string  `gorm:"column:is_nullable"`
	DefaultValue *string `gorm:"column:column_default"`
	Extra        string  `gorm:"column:extra"`
}

// MigrateAbilityModelCollation makes abilities.model use its MySQL character
// set's binary collation. Other supported databases require no schema change.
func MigrateAbilityModelCollation() error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	if !common.UsingMySQL.Load() {
		return nil
	}
	if !DB.Migrator().HasTable(&Ability{}) {
		logger.Logger.Debug("Abilities table not found, skipping model collation migration")
		return nil
	}

	if err := migrateAbilityModelCollationMySQL(DB); err != nil {
		return errors.Wrap(err, "migrate abilities.model MySQL collation")
	}
	return nil
}

// migrateAbilityModelCollationMySQL validates the existing column metadata and
// changes its collation to the binary collation for the same character set.
func migrateAbilityModelCollationMySQL(db *gorm.DB) error {
	column, err := loadMySQLAbilityModelColumn(db)
	if err != nil {
		return errors.Wrap(err, "load abilities.model metadata")
	}

	columnType, err := validateMySQLAbilityModelColumnType(column.ColumnType)
	if err != nil {
		return errors.Wrap(err, "validate abilities.model column type")
	}
	characterSet, err := validateMySQLIdentifier(column.CharacterSet, "character set")
	if err != nil {
		return errors.Wrap(err, "validate abilities.model character set")
	}
	targetCollation := "binary"
	if !strings.EqualFold(characterSet, "binary") {
		targetCollation = characterSet + "_bin"
	}
	if _, err := validateMySQLIdentifier(targetCollation, "collation"); err != nil {
		return errors.Wrap(err, "validate target abilities.model collation")
	}

	nullability, err := validateMySQLAbilityModelNullability(column.IsNullable)
	if err != nil {
		return errors.Wrap(err, "validate abilities.model nullability")
	}
	if column.DefaultValue != nil {
		return errors.Errorf("abilities.model has unsupported default value %q", *column.DefaultValue)
	}
	if strings.TrimSpace(column.Extra) != "" {
		return errors.Errorf("abilities.model has unsupported extra metadata %q", column.Extra)
	}
	if strings.EqualFold(column.Collation, targetCollation) {
		logger.Logger.Debug("Abilities.model already uses binary collation",
			zap.String("collation", targetCollation))
		return nil
	}

	exists, err := mysqlCollationExists(db, characterSet, targetCollation)
	if err != nil {
		return errors.Wrap(err, "verify target abilities.model collation")
	}
	if !exists {
		return errors.Errorf("MySQL collation %q does not exist for character set %q", targetCollation, characterSet)
	}

	alterSQL := fmt.Sprintf(
		"ALTER TABLE %s MODIFY COLUMN %s %s CHARACTER SET %s COLLATE %s %s",
		quoteIdentifier(db, "abilities"),
		quoteIdentifier(db, "model"),
		columnType,
		characterSet,
		targetCollation,
		nullability,
	)
	if err := db.Exec(alterSQL).Error; err != nil {
		return errors.Wrap(err, "alter abilities.model to binary collation")
	}

	logger.Logger.Info("Migrated abilities.model to binary collation",
		zap.String("character_set", characterSet),
		zap.String("collation", targetCollation))
	return nil
}

// loadMySQLAbilityModelColumn returns the metadata needed to safely reproduce
// abilities.model in an ALTER TABLE statement.
func loadMySQLAbilityModelColumn(db *gorm.DB) (mysqlAbilityModelColumn, error) {
	var column mysqlAbilityModelColumn
	query := `SELECT COLUMN_TYPE AS column_type,
		CHARACTER_SET_NAME AS character_set_name,
		COLLATION_NAME AS collation_name,
		IS_NULLABLE AS is_nullable,
		COLUMN_DEFAULT AS column_default,
		EXTRA AS extra
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`
	result := db.Raw(query, "abilities", "model").Scan(&column)
	if result.Error != nil {
		return mysqlAbilityModelColumn{}, errors.Wrap(result.Error, "query MySQL information_schema for abilities.model")
	}
	if result.RowsAffected != 1 || strings.TrimSpace(column.ColumnType) == "" {
		return mysqlAbilityModelColumn{}, errors.Errorf("expected one abilities.model metadata row, got %d", result.RowsAffected)
	}
	return column, nil
}

// validateMySQLAbilityModelColumnType accepts only bounded CHAR or VARCHAR
// definitions and returns a normalized value safe for DDL interpolation.
func validateMySQLAbilityModelColumnType(columnType string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(columnType))
	matches := mysqlAbilityModelColumnTypePattern.FindStringSubmatch(trimmed)
	if matches == nil {
		return "", errors.Errorf("unsupported MySQL column type %q", columnType)
	}
	length, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", errors.Wrap(err, "parse MySQL character column length")
	}
	if length < 1 || length > 16_383 {
		return "", errors.Errorf("MySQL character column length %d is outside the supported range", length)
	}
	if matches[1] == "var" {
		return fmt.Sprintf("varchar(%d)", length), nil
	}
	return fmt.Sprintf("char(%d)", length), nil
}

// validateMySQLIdentifier accepts a conservative ASCII identifier and returns
// its normalized lowercase representation for safe DDL interpolation.
func validateMySQLIdentifier(identifier string, kind string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(identifier))
	if normalized == "" || len(normalized) > 64 {
		return "", errors.Errorf("invalid MySQL %s identifier %q", kind, identifier)
	}
	for _, char := range normalized {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return "", errors.Errorf("invalid MySQL %s identifier %q", kind, identifier)
	}
	return normalized, nil
}

// validateMySQLAbilityModelNullability returns the normalized nullability clause
// allowed in the abilities.model ALTER TABLE statement.
func validateMySQLAbilityModelNullability(nullable string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(nullable)) {
	case "NO":
		return "NOT NULL", nil
	case "YES":
		return "NULL", nil
	default:
		return "", errors.Errorf("invalid MySQL nullability %q", nullable)
	}
}

// mysqlCollationExists reports whether MySQL exposes the requested collation for
// the requested character set.
func mysqlCollationExists(db *gorm.DB, characterSet string, collation string) (bool, error) {
	type result struct {
		Count int `gorm:"column:count"`
	}
	var found result
	query := `SELECT COUNT(*) AS count FROM information_schema.collations
		WHERE character_set_name = ? AND collation_name = ?`
	if err := db.Raw(query, characterSet, collation).Scan(&found).Error; err != nil {
		return false, errors.Wrap(err, "query MySQL information_schema for collation")
	}
	return found.Count == 1, nil
}
