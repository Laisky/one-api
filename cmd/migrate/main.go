package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/cmd/migrate/internal"
	"github.com/Laisky/one-api/common/logger"
)

const (
	version = "1.0.0"
	usage   = `One API Database Migration Tool v%s

DESCRIPTION:
	This tool helps migrate data between different database types supported by One API:
	- SQLite
	- MySQL
	- PostgreSQL

USAGE:
	%s [OPTIONS]

EXAMPLES:
	# Migrate from SQLite to MySQL with custom concurrency
	%s -source-dsn="sqlite://./one-api.db" -target-dsn="mysql://user:pass@localhost:3306/oneapi" -workers=8 -batch-size=2000

	# Migrate from MySQL to PostgreSQL
	%s -source-dsn="mysql://user:pass@localhost:3306/oneapi" -target-dsn="postgres://user:pass@localhost/oneapi?sslmode=disable"

	# Dry run to validate migration without making changes
	%s -source-dsn="sqlite://./one-api.db" -target-dsn="mysql://user:pass@localhost:3306/oneapi" -dry-run

	# Re-run migration safely (idempotent - handles existing data automatically)
	%s -source-dsn="./one-api.db" -target-dsn="postgres://user:pass@localhost/oneapi"

OPTIONS:
`
)

var (
	sourceDSN      = flag.String("source-dsn", "", "Source database connection string (e.g., sqlite://./db.sqlite, postgres://user:pass@host/db)")
	targetDSN      = flag.String("target-dsn", "", "Target database connection string (e.g., postgres://user:pass@host/db, mysql://user:pass@host/db)")
	dryRun         = flag.Bool("dry-run", false, "Perform validation without making changes")
	validateOnly   = flag.Bool("validate-only", false, "Only validate connections and compatibility, don't migrate")
	showPlan       = flag.Bool("show-plan", false, "Show migration plan and exit")
	verbose        = flag.Bool("verbose", false, "Enable verbose logging")
	showHelp       = flag.Bool("h", false, "Show this help message")
	showVersion    = flag.Bool("v", false, "Show version information")
	skipValidation = flag.Bool("skip-validation", false, "Skip pre-migration validation (not recommended)")
	workers        = flag.Int("workers", 4, "Number of concurrent workers for batch processing")
	batchSize      = flag.Int("batch-size", 1000, "Number of records to process in each batch")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, version, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("One API Database Migration Tool v%s\n", version)
		os.Exit(0)
	}

	// Setup logging
	logger.SetupLogger()
	if *verbose {
		logger.Logger.Info("Verbose logging enabled")
	}

	// Validate required parameters
	if err := validateFlags(); err != nil {
		logger.Logger.Error("invalid flags", zap.Error(err))
		flag.Usage()
		os.Exit(1)
	}

	// Create migration context
	ctx := context.Background()

	// Extract database types from DSNs
	sourceType, err := internal.ExtractDatabaseTypeFromDSN(*sourceDSN)
	if err != nil {
		logger.Logger.Error("failed to determine source database type", zap.Error(err))
		flag.Usage()
		os.Exit(1)
	}

	var targetType string
	if *targetDSN != "" {
		targetType, err = internal.ExtractDatabaseTypeFromDSN(*targetDSN)
		if err != nil {
			logger.Logger.Error("failed to determine target database type", zap.Error(err))
			flag.Usage()
			os.Exit(1)
		}
	}

	migrator := &internal.Migrator{
		SourceType: sourceType,
		SourceDSN:  *sourceDSN,
		TargetType: targetType,
		TargetDSN:  *targetDSN,
		DryRun:     *dryRun,
		Verbose:    *verbose,
		Workers:    *workers,
		BatchSize:  *batchSize,
	}

	// Handle different operation modes
	if *showPlan {
		if err := showMigrationPlan(migrator); err != nil {
			logger.Logger.Fatal("failed to generate migration plan", zap.Error(err))
		}
		return
	}

	if *validateOnly {
		if err := migrator.ValidateOnly(ctx); err != nil {
			logger.Logger.Fatal("validation failed", zap.Error(err))
		}
		logger.Logger.Info("Validation completed successfully")
		return
	}

	// Run pre-migration validation unless skipped
	if !*skipValidation {
		if err := runPreMigrationValidation(migrator); err != nil {
			logger.Logger.Fatal("pre-migration validation failed", zap.Error(err))
		}
	}

	// Run migration
	if err := migrator.Migrate(ctx); err != nil {
		logger.Logger.Fatal("migration failed", zap.Error(err))
	}

	if *dryRun {
		logger.Logger.Info("Dry run completed successfully - no changes were made")
	} else {
		logger.Logger.Info("Migration completed successfully")
	}
}

func validateFlags() error {
	// Source DSN is always required
	if *sourceDSN == "" {
		return errors.Errorf("source-dsn is required")
	}

	// Target DSN is only required for actual migration
	if !*showPlan && !*validateOnly {
		if *targetDSN == "" {
			return errors.Errorf("target-dsn is required")
		}
	}

	// Validate DSN formats and extract types
	if err := internal.ValidateDSN(*sourceDSN); err != nil {
		return errors.Wrapf(err, "invalid source DSN")
	}

	sourceType, err := internal.ExtractDatabaseTypeFromDSN(*sourceDSN)
	if err != nil {
		return errors.Wrapf(err, "unable to determine source database type")
	}

	if *targetDSN != "" {
		if err := internal.ValidateDSN(*targetDSN); err != nil {
			return errors.Wrapf(err, "invalid target DSN")
		}

		targetType, err := internal.ExtractDatabaseTypeFromDSN(*targetDSN)
		if err != nil {
			return errors.Wrapf(err, "unable to determine target database type")
		}

		// Check if source and target are the same
		if sourceType == targetType && *sourceDSN == *targetDSN {
			return errors.Errorf("source and target cannot be the same")
		}
	}

	// Validate flag combinations
	if *dryRun && *validateOnly {
		return errors.Errorf("cannot use both --dry-run and --validate-only")
	}

	if *showPlan && (*dryRun || *validateOnly) {
		return errors.Errorf("--show-plan cannot be used with other operation modes")
	}

	return nil
}

// showMigrationPlan displays the migration plan
func showMigrationPlan(migrator *internal.Migrator) error {
	plan, err := migrator.GetMigrationPlan()
	if err != nil {
		return errors.Wrapf(err, "failed to get migration plan")
	}

	fmt.Printf("\n=== Migration Plan ===\n")
	fmt.Printf("Source: %s\n", plan.SourceType)
	fmt.Printf("Target: %s\n", plan.TargetType)
	fmt.Printf("Total Records: %d\n\n", plan.TotalRecords)

	fmt.Printf("Tables to migrate:\n")
	for _, table := range plan.Tables {
		if table.Exists {
			fmt.Printf("  ✓ %s (%d records)\n", table.Name, table.RecordCount)
		} else {
			fmt.Printf("  ✗ %s (table not found)\n", table.Name)
		}
	}

	fmt.Printf("\nUse --dry-run to test the migration without making changes.\n")
	return nil
}

// runPreMigrationValidation runs comprehensive pre-migration validation
func runPreMigrationValidation(migrator *internal.Migrator) error {
	logger.Logger.Info("Running pre-migration validation...")

	validator := internal.NewPreMigrationValidator(migrator)
	result, err := validator.ValidateAll()
	if err != nil {
		return errors.Wrapf(err, "validation process failed")
	}

	// Display warnings
	if len(result.Warnings) > 0 {
		logger.Logger.Info("=== Validation Warnings ===")
		for _, warning := range result.Warnings {
			logger.Logger.Warn(warning)
		}
		fmt.Println()
	}

	// Display errors
	if len(result.Errors) > 0 {
		logger.Logger.Info("=== Validation Errors ===")
		for _, error := range result.Errors {
			logger.Logger.Error(error)
		}
		fmt.Println()
	}

	if !result.Valid {
		return errors.Errorf("validation failed with %d errors", len(result.Errors))
	}

	logger.Logger.Info("Pre-migration validation completed successfully")
	return nil
}
