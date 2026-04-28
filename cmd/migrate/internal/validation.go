package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/logger"
)

// ValidationResult represents the result of a validation check
type ValidationResult struct {
	Valid    bool
	Warnings []string
	Errors   []string
}

// PreMigrationValidator performs comprehensive pre-migration validation
type PreMigrationValidator struct {
	migrator *Migrator
}

// NewPreMigrationValidator creates a new validator
func NewPreMigrationValidator(migrator *Migrator) *PreMigrationValidator {
	return &PreMigrationValidator{
		migrator: migrator,
	}
}

// ValidateAll performs all validation checks
func (v *PreMigrationValidator) ValidateAll() (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Warnings: make([]string, 0),
		Errors:   make([]string, 0),
	}

	// Connect to databases once for all validation steps
	if err := v.migrator.connectDatabases(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to connect to databases: %v", err))
		result.Valid = false
		return result, nil
	}
	defer v.migrator.closeDatabases()

	// Validate database connections
	if err := v.validateConnections(result); err != nil {
		return result, errors.Wrapf(err, "connection validation failed")
	}

	// Validate source database
	if err := v.validateSourceDatabase(result); err != nil {
		return result, errors.Wrapf(err, "source database validation failed")
	}

	// Validate target database
	if err := v.validateTargetDatabase(result); err != nil {
		return result, errors.Wrapf(err, "target database validation failed")
	}

	// Validate migration compatibility
	if err := v.validateMigrationCompatibility(result); err != nil {
		return result, errors.Wrapf(err, "migration compatibility validation failed")
	}

	// Check for potential issues
	v.checkPotentialIssues(result)

	// Generate backup recommendations
	v.generateBackupRecommendations(result)

	// Set overall validity
	result.Valid = len(result.Errors) == 0

	return result, nil
}

// validateConnections validates database connections
func (v *PreMigrationValidator) validateConnections(result *ValidationResult) error {
	logger.Logger.Info("Validating database connections...")

	// Validate source connection
	if err := v.migrator.sourceConn.ValidateConnection(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Source database connection invalid: %v", err))
	}

	// Validate target connection
	if err := v.migrator.targetConn.ValidateConnection(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Target database connection invalid: %v", err))
	}

	return nil
}

// validateSourceDatabase validates the source database
func (v *PreMigrationValidator) validateSourceDatabase(result *ValidationResult) error {
	logger.Logger.Info("Validating source database...")

	// Check if source database has expected tables
	expectedTables := []string{"users", "tokens", "channels", "options", "redemptions", "abilities", "logs", "user_request_costs", "traces"}

	for _, tableName := range expectedTables {
		exists, err := v.migrator.sourceConn.TableExists(tableName)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Could not check if table %s exists: %v", tableName, err))
			continue
		}

		if !exists {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Table %s does not exist in source database", tableName))
		} else {
			// Check if table has data
			count, err := v.migrator.sourceConn.GetRowCount(tableName)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Could not count rows in table %s: %v", tableName, err))
			} else if count == 0 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Table %s is empty", tableName))
			}
		}
	}

	return nil
}

// validateTargetDatabase validates the target database
func (v *PreMigrationValidator) validateTargetDatabase(result *ValidationResult) error {
	logger.Logger.Info("Validating target database...")

	// Check if target database is empty or has conflicting data
	tables, err := v.migrator.targetConn.GetTableNames()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Could not get table names from target database: %v", err))
		return nil
	}

	hasData := false
	for _, tableName := range tables {
		count, err := v.migrator.targetConn.GetRowCount(tableName)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Could not count rows in target table %s: %v", tableName, err))
			continue
		}

		if count > 0 {
			hasData = true
			result.Warnings = append(result.Warnings, fmt.Sprintf("Target table %s already contains %d records", tableName, count))
		}
	}

	if hasData {
		result.Warnings = append(result.Warnings, "Target database is not empty. Migration may overwrite existing data.")
	}

	return nil
}

// validateMigrationCompatibility validates migration compatibility
func (v *PreMigrationValidator) validateMigrationCompatibility(result *ValidationResult) error {
	logger.Logger.Info("Validating migration compatibility...")

	// Check for known compatibility issues
	sourceType := v.migrator.sourceConn.Type
	targetType := v.migrator.targetConn.Type

	// SQLite to MySQL/PostgreSQL specific checks
	if sourceType == "sqlite" && (targetType == "mysql" || targetType == "postgres") {
		result.Warnings = append(result.Warnings, "Migrating from SQLite: Some data types may be converted automatically")
	}

	// MySQL to PostgreSQL specific checks
	if sourceType == "mysql" && targetType == "postgres" {
		result.Warnings = append(result.Warnings, "Migrating from MySQL to PostgreSQL: Case sensitivity and data type differences may apply")
	}

	// PostgreSQL to MySQL specific checks
	if sourceType == "postgres" && targetType == "mysql" {
		result.Warnings = append(result.Warnings, "Migrating from PostgreSQL to MySQL: Some advanced data types may not be fully compatible")
	}

	return nil
}

// checkPotentialIssues checks for potential migration issues
func (v *PreMigrationValidator) checkPotentialIssues(result *ValidationResult) {
	logger.Logger.Info("Checking for potential issues...")

	// Check for large datasets
	for _, tableInfo := range TableMigrationOrder {
		exists, err := v.migrator.sourceConn.TableExists(tableInfo.Name)
		if err != nil || !exists {
			continue
		}

		count, err := v.migrator.sourceConn.GetRowCount(tableInfo.Name)
		if err != nil {
			continue
		}

		// Warn about large tables
		if count > 100000 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Table %s has %d records - migration may take significant time", tableInfo.Name, count))
		}

		// Warn about very large tables
		if count > 1000000 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Table %s has %d records - consider using batch processing or off-peak hours", tableInfo.Name, count))
		}
	}

	// Check disk space (basic check for SQLite files)
	if v.migrator.SourceType == "sqlite" {
		if stat, err := os.Stat(v.migrator.SourceDSN); err == nil {
			sizeGB := float64(stat.Size()) / (1024 * 1024 * 1024)
			if sizeGB > 1.0 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Source SQLite database is %.2f GB - ensure sufficient disk space and time for migration", sizeGB))
			}
		}
	}
}

// generateBackupRecommendations generates backup recommendations
func (v *PreMigrationValidator) generateBackupRecommendations(result *ValidationResult) {
	logger.Logger.Info("Generating backup recommendations...")

	recommendations := []string{
		"IMPORTANT: Always backup your databases before migration",
		"Test the migration process on a copy of your data first",
		"Consider running the migration during off-peak hours",
		"Monitor the migration process and be prepared to rollback if needed",
	}

	// Database-specific recommendations
	switch v.migrator.SourceType {
	case "sqlite":
		recommendations = append(recommendations, "For SQLite: Simply copy the database file to create a backup")
	case "mysql":
		recommendations = append(recommendations, "For MySQL: Use mysqldump to create a backup")
	case "postgres":
		recommendations = append(recommendations, "For PostgreSQL: Use pg_dump to create a backup")
	}

	switch v.migrator.TargetType {
	case "sqlite":
		recommendations = append(recommendations, "Target SQLite: Ensure the target directory is writable")
	case "mysql":
		recommendations = append(recommendations, "Target MySQL: Ensure the target database exists and user has proper permissions")
	case "postgres":
		recommendations = append(recommendations, "Target PostgreSQL: Ensure the target database exists and user has proper permissions")
	}

	for _, rec := range recommendations {
		result.Warnings = append(result.Warnings, rec)
	}
}

// ExtractDatabaseTypeFromDSN extracts the database type from a DSN
func ExtractDatabaseTypeFromDSN(dsn string) (string, error) {
	dsn = strings.TrimSpace(dsn)

	if strings.HasPrefix(dsn, "sqlite://") {
		return "sqlite", nil
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return "postgres", nil
	}
	if strings.HasPrefix(dsn, "mysql://") {
		return "mysql", nil
	}

	// For backward compatibility, also check for common patterns without schemes
	if strings.Contains(dsn, "@tcp(") || strings.Contains(dsn, "@unix(") {
		return "mysql", nil
	}

	// If it looks like a file path or :memory:, assume SQLite
	if dsn == ":memory:" || (!strings.Contains(dsn, "://") && !strings.Contains(dsn, "@")) {
		return "sqlite", nil
	}

	return "", errors.Wrapf(nil, "unable to determine database type from DSN: %s", dsn)
}

// ValidateDSN validates a database connection string
func ValidateDSN(dsn string) error {
	dbType, err := ExtractDatabaseTypeFromDSN(dsn)
	if err != nil {
		return errors.Wrapf(err, "failed to extract database type from DSN")
	}

	switch strings.ToLower(dbType) {
	case "sqlite":
		return validateSQLiteDSN(dsn)
	case "mysql":
		return validateMySQLDSN(dsn)
	case "postgres", "postgresql":
		return validatePostgresDSN(dsn)
	default:
		return errors.Wrapf(nil, "unsupported database type: %s", dbType)
	}
}

// validateSQLiteDSN validates SQLite DSN
func validateSQLiteDSN(dsn string) error {
	// Handle sqlite:// scheme
	path := dsn
	if after, ok := strings.CutPrefix(dsn, "sqlite://"); ok {
		path = after
	}

	// Remove query parameters for path validation
	path = strings.Split(path, "?")[0]

	if path == ":memory:" {
		return nil // In-memory database is valid
	}

	// Check if directory exists for file-based SQLite
	dir := filepath.Dir(path)
	if dir != "." {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return errors.Wrapf(err, "directory does not exist: %s", dir)
		}
	}

	return nil
}

// validateMySQLDSN validates MySQL DSN
func validateMySQLDSN(dsn string) error {
	// Handle mysql:// scheme
	if strings.HasPrefix(dsn, "mysql://") {
		// For mysql:// scheme, we expect: mysql://user:password@host:port/database
		if !strings.Contains(dsn, "@") || !strings.Contains(dsn, "/") {
			return errors.Wrapf(nil, "invalid MySQL DSN format - expected format: mysql://user:password@host:port/database")
		}
		return nil
	}

	// Basic format check for traditional MySQL DSN
	if !strings.Contains(dsn, "@tcp(") && !strings.Contains(dsn, "@unix(") {
		return errors.Wrapf(nil, "invalid MySQL DSN format - expected format: user:password@tcp(host:port)/database or mysql://user:password@host:port/database")
	}
	return nil
}

// validatePostgresDSN validates PostgreSQL DSN
func validatePostgresDSN(dsn string) error {
	// Basic format check for PostgreSQL DSN
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return errors.Wrapf(nil, "invalid PostgreSQL DSN format - expected format: postgres://user:password@host:port/database")
	}
	return nil
}
