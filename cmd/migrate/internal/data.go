package internal

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// BatchJob represents a batch processing job
type BatchJob struct {
	TableInfo TableInfo
	Offset    int64
	Limit     int
	JobID     int
}

// BatchResult represents the result of a batch processing job
type BatchResult struct {
	JobID       int
	RecordCount int64
	Error       error
}

// TableMigrationOrder defines the order in which tables should be migrated
// to respect foreign key constraints
var TableMigrationOrder = []TableInfo{
	{"users", &model.User{}},
	{"options", &model.Option{}},
	{"tokens", &model.Token{}},
	{"channels", &model.Channel{}},
	{"redemptions", &model.Redemption{}},
	{"abilities", &model.Ability{}},
	{"logs", &model.Log{}},
	{"user_request_costs", &model.UserRequestCost{}},
	{"traces", &model.Trace{}},
}

// TableInfo holds information about a table and its corresponding model
type TableInfo struct {
	Name  string
	Model any
}

// migrateData performs the actual data migration
func (m *Migrator) migrateData(ctx context.Context, stats *MigrationStats) error {
	logger.Logger.Info("Starting data migration...")

	for _, tableInfo := range TableMigrationOrder {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := m.migrateTable(ctx, tableInfo, stats); err != nil {
			stats.Errors = append(stats.Errors, errors.Wrapf(err, "failed to migrate table %s", tableInfo.Name))
			logger.Logger.Error("Failed to migrate table",
				zap.String("table", tableInfo.Name),
				zap.Error(err))
			continue
		}

		stats.TablesDone++
		logger.Logger.Info("Successfully migrated table",
			zap.String("table", tableInfo.Name),
			zap.Int("tables_done", stats.TablesDone),
			zap.Int("tables_total", stats.TablesTotal))
	}

	logger.Logger.Info("Data migration completed")
	return nil
}

// migrateTable migrates data for a specific table using concurrent workers
func (m *Migrator) migrateTable(ctx context.Context, tableInfo TableInfo, stats *MigrationStats) error {
	// Check if table exists in source
	exists, err := m.sourceConn.TableExists(tableInfo.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to check if table exists")
	}
	if !exists {
		logger.Logger.Warn("Table does not exist in source database, skipping", zap.String("table", tableInfo.Name))
		return nil
	}

	// Get total count for progress tracking
	totalCount, err := m.sourceConn.GetRowCount(tableInfo.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get row count")
	}

	if totalCount == 0 {
		logger.Logger.Info("Table is empty, skipping", zap.String("table", tableInfo.Name))
		return nil
	}

	logger.Logger.Info("Migrating table",
		zap.String("table", tableInfo.Name),
		zap.Int64("total_records", totalCount),
		zap.Int("workers", m.Workers),
		zap.Int("batch_size", m.BatchSize))

	// Use concurrent processing for better performance
	if m.Workers > 1 {
		return m.migrateTableConcurrent(ctx, tableInfo, totalCount, stats)
	} else {
		return m.migrateTableSequential(ctx, tableInfo, totalCount, stats)
	}
}

// migrateTableSequential migrates data sequentially (single-threaded)
func (m *Migrator) migrateTableSequential(ctx context.Context, tableInfo TableInfo, totalCount int64, stats *MigrationStats) error {
	var offset int64 = 0
	var migratedCount int64 = 0
	var lastProgressReport int64 = 0

	for offset < totalCount {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		batchCount, err := m.migrateBatch(tableInfo, offset, m.BatchSize)
		if err != nil {
			return errors.Wrapf(err, "failed to migrate batch at offset %d", offset)
		}

		migratedCount += batchCount
		offset += int64(m.BatchSize)
		atomic.AddInt64(&stats.RecordsDone, batchCount)

		// Show progress every 10% or every 10,000 records, whichever is less frequent
		progressThreshold := max(totalCount/10, 10000)

		if m.Verbose && (migratedCount-lastProgressReport >= progressThreshold || migratedCount == totalCount) {
			progress := float64(migratedCount) / float64(totalCount) * 100
			logger.Logger.Info("Table migration progress",
				zap.String("table", tableInfo.Name),
				zap.Int64("migrated", migratedCount),
				zap.Int64("total", totalCount),
				zap.Float64("progress_percent", progress))
			lastProgressReport = migratedCount
		}

		// Break if we've processed all records
		if batchCount < int64(m.BatchSize) {
			break
		}
	}

	logger.Logger.Info("Table migration completed",
		zap.String("table", tableInfo.Name),
		zap.Int64("migrated", migratedCount))
	return nil
}

// migrateTableConcurrent migrates data using concurrent workers
func (m *Migrator) migrateTableConcurrent(ctx context.Context, tableInfo TableInfo, totalCount int64, stats *MigrationStats) error {
	// Create job and result channels
	jobs := make(chan BatchJob, m.Workers*2)
	results := make(chan BatchResult, m.Workers*2)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < m.Workers; i++ {
		wg.Go(func() {
			m.batchWorker(ctx, jobs, results)
		})
	}

	// Start result collector
	var collectorWg sync.WaitGroup
	var migratedCount int64
	var lastProgressReport int64
	collectorWg.Go(func() {
		for result := range results {
			if result.Error != nil {
				logger.Logger.Error("Batch job failed",
					zap.Int("job_id", result.JobID),
					zap.Error(result.Error))
				continue
			}

			atomic.AddInt64(&migratedCount, result.RecordCount)
			atomic.AddInt64(&stats.RecordsDone, result.RecordCount)

			// Show progress
			currentCount := atomic.LoadInt64(&migratedCount)
			progressThreshold := max(totalCount/10, 10000)

			if m.Verbose && (currentCount-lastProgressReport >= progressThreshold || currentCount >= totalCount) {
				progress := float64(currentCount) / float64(totalCount) * 100
				logger.Logger.Info("Table migration progress",
					zap.String("table", tableInfo.Name),
					zap.Int64("migrated", currentCount),
					zap.Int64("total", totalCount),
					zap.Float64("progress_percent", progress))
				lastProgressReport = currentCount
			}
		}
	})

	// Generate jobs
	jobID := 0
	for offset := int64(0); offset < totalCount; offset += int64(m.BatchSize) {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			close(results)
			collectorWg.Wait()
			return ctx.Err()
		case jobs <- BatchJob{
			TableInfo: tableInfo,
			Offset:    offset,
			Limit:     m.BatchSize,
			JobID:     jobID,
		}:
			jobID++
		}
	}

	// Close jobs channel and wait for workers to finish
	close(jobs)
	wg.Wait()
	close(results)
	collectorWg.Wait()

	finalCount := atomic.LoadInt64(&migratedCount)
	logger.Logger.Info("Table migration completed",
		zap.String("table", tableInfo.Name),
		zap.Int64("migrated", finalCount))
	return nil
}

// batchWorker processes batch jobs concurrently
func (m *Migrator) batchWorker(ctx context.Context, jobs <-chan BatchJob, results chan<- BatchResult) {
	for job := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		count, err := m.migrateBatch(job.TableInfo, job.Offset, job.Limit)
		results <- BatchResult{
			JobID:       job.JobID,
			RecordCount: count,
			Error:       err,
		}
	}
}

// schemaCache memoizes parsed GORM schemas across migrateBatch invocations.
// schema.Parse caches into the provided sync.Map, so reusing one map avoids
// re-parsing reflective info on every batch.
var schemaCache sync.Map

// primaryKeyOrderClause returns an SQL ORDER BY fragment listing the
// quoted primary key columns of the given model, in declaration order, all
// ascending. Used to make LIMIT/OFFSET pagination deterministic across a
// concurrently-written source.
func primaryKeyOrderClause(db *gorm.DB, model any) (string, error) {
	s, err := schema.Parse(model, &schemaCache, db.NamingStrategy)
	if err != nil {
		return "", errors.Wrapf(err, "parse model schema")
	}
	if len(s.PrimaryFields) == 0 {
		return "", errors.Errorf("model %T has no primary key", model)
	}
	cols := make([]string, 0, len(s.PrimaryFields))
	for _, f := range s.PrimaryFields {
		cols = append(cols, db.Statement.Quote(f.DBName))
	}
	return strings.Join(cols, ", "), nil
}

// migrateBatch migrates a batch of records for a specific table
func (m *Migrator) migrateBatch(tableInfo TableInfo, offset int64, limit int) (int64, error) {
	// Create a slice to hold the batch data
	modelType := reflect.TypeOf(tableInfo.Model).Elem()
	sliceType := reflect.SliceOf(modelType)
	batch := reflect.New(sliceType).Interface()

	// Determine the source's primary key columns. Without ORDER BY, LIMIT/OFFSET
	// produces inconsistent slices on a concurrently-written source (PG row
	// order is not stable), causing batches to overlap and skip rows — which
	// silently loses records. Ordering by the PK gives stable, non-overlapping
	// windows even while new rows are being appended.
	orderClause, err := primaryKeyOrderClause(m.sourceConn.DB, tableInfo.Model)
	if err != nil {
		return 0, errors.Wrapf(err, "resolve primary key for %s", tableInfo.Name)
	}

	// Fetch batch from source database
	query := m.sourceConn.DB.Order(orderClause).Limit(limit).Offset(int(offset))
	if err := query.Find(batch).Error; err != nil {
		return 0, errors.Wrapf(err, "failed to fetch batch from source")
	}

	// Get the actual slice value
	batchValue := reflect.ValueOf(batch).Elem()
	batchLen := batchValue.Len()

	if batchLen == 0 {
		return 0, nil
	}

	// Skip insertion in dry run mode
	if m.DryRun {
		return int64(batchLen), nil
	}

	// Insert batch into target database with conflict resolution
	if err := m.insertBatchWithConflictResolution(batch, tableInfo); err != nil {
		return 0, errors.Wrapf(err, "failed to insert batch into target")
	}

	return int64(batchLen), nil
}

// insertBatchWithConflictResolution inserts a batch with conflict resolution
func (m *Migrator) insertBatchWithConflictResolution(batch any, tableInfo TableInfo) error {
	// First try a simple insert for better performance
	err := m.targetConn.DB.Create(batch).Error
	if err == nil {
		return nil // Success - no conflicts
	}

	// If we get a conflict error, use upsert approach
	if m.isConflictError(err) {
		if m.Verbose {
			logger.Logger.Info("Conflict detected, switching to upsert mode", zap.String("table", tableInfo.Name))
		}
		return m.upsertBatch(batch, tableInfo)
	}

	// For non-conflict errors, return the original error
	return errors.WithStack(err)
}

// isConflictError checks if the error is a primary key or unique constraint violation
func (m *Migrator) isConflictError(err error) bool {
	errStr := err.Error()
	// Check for common conflict error patterns across different databases
	conflictPatterns := []string{
		// PostgreSQL
		"duplicate key value violates unique constraint",
		"violates unique constraint",
		// SQLite
		"UNIQUE constraint failed",
		"constraint failed: UNIQUE",
		// MySQL
		"Duplicate entry",
		"duplicate key",
		"Duplicate key name",
		// General patterns
		"already exists",
		"constraint violation",
	}

	for _, pattern := range conflictPatterns {
		if contains(errStr, pattern) {
			return true
		}
	}
	return false
}

// upsertBatch performs upsert operation for conflicting records
func (m *Migrator) upsertBatch(batch any, tableInfo TableInfo) error {
	// Get the slice value
	batchValue := reflect.ValueOf(batch).Elem()
	batchLen := batchValue.Len()

	successCount := 0
	errorCount := 0
	var firstErr error
	const maxLoggedErrors = 3
	loggedErrors := 0

	// Process each record individually for upsert
	for i := range batchLen {
		record := batchValue.Index(i).Addr().Interface()

		// Use GORM's Save method which performs INSERT or UPDATE
		result := m.targetConn.DB.Save(record)
		if result.Error != nil {
			errorCount++
			if firstErr == nil {
				firstErr = result.Error
			}
			// Always log up to a few failure samples so silent data loss is
			// visible. Verbose mode logs every failure.
			if m.Verbose || loggedErrors < maxLoggedErrors {
				logger.Logger.Warn("Failed to upsert record",
					zap.Int("record_index", i+1),
					zap.String("table", tableInfo.Name),
					zap.Error(result.Error))
				loggedErrors++
			}
			// Continue with other records instead of failing the entire batch
		} else {
			successCount++
		}
	}

	if errorCount > 0 {
		atomic.AddInt64(&m.upsertFailures, int64(errorCount))
		logger.Logger.Warn("Table upsert batch completed with errors",
			zap.String("table", tableInfo.Name),
			zap.Int("successful", successCount),
			zap.Int("failed", errorCount),
			zap.Error(firstErr))
	}

	return nil
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr)))
}

// containsSubstring is a helper function for substring checking
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// validateResults validates the migration results by comparing record counts
func (m *Migrator) validateResults(stats *MigrationStats) error {
	logger.Logger.Info("Validating migration results...")

	var validationErrors []error

	for _, tableInfo := range TableMigrationOrder {
		// Check if table exists in source
		sourceExists, err := m.sourceConn.TableExists(tableInfo.Name)
		if err != nil {
			validationErrors = append(validationErrors, errors.Wrapf(err, "failed to check source table %s", tableInfo.Name))
			continue
		}

		if !sourceExists {
			continue // Skip tables that don't exist in source
		}

		// Get source count
		sourceCount, err := m.sourceConn.GetRowCount(tableInfo.Name)
		if err != nil {
			validationErrors = append(validationErrors, errors.Wrapf(err, "failed to get source count for %s", tableInfo.Name))
			continue
		}

		// Get target count
		targetCount, err := m.targetConn.GetRowCount(tableInfo.Name)
		if err != nil {
			validationErrors = append(validationErrors, errors.Wrapf(err, "failed to get target count for %s", tableInfo.Name))
			continue
		}

		// Compare counts
		if sourceCount != targetCount {
			validationErrors = append(validationErrors, errors.Wrapf(nil, "record count mismatch for table %s: source=%d, target=%d", tableInfo.Name, sourceCount, targetCount))
		} else {
			if m.Verbose {
				logger.Logger.Info("Table validation passed",
					zap.String("table", tableInfo.Name),
					zap.Int64("records", sourceCount))
			}
		}
	}

	if len(validationErrors) > 0 {
		logger.Logger.Error("Migration validation failed:")
		for _, err := range validationErrors {
			logger.Logger.Error("Migration validation error", zap.Error(err))
		}
		return errors.Wrapf(nil, "migration validation failed with %d errors", len(validationErrors))
	}

	logger.Logger.Info("Migration validation completed successfully")
	return nil
}

// ExportData exports data from source database to a structured format
func (m *Migrator) ExportData(ctx context.Context) (map[string]any, error) {
	logger.Logger.Info("Exporting data from source database...")

	exportData := make(map[string]any)

	for _, tableInfo := range TableMigrationOrder {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Check if table exists
		exists, err := m.sourceConn.TableExists(tableInfo.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to check if table %s exists", tableInfo.Name)
		}

		if !exists {
			logger.Logger.Warn("Table does not exist in source database, skipping", zap.String("table", tableInfo.Name))
			continue
		}

		// Export table data
		tableData, err := m.exportTable(tableInfo)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to export table %s", tableInfo.Name)
		}

		exportData[tableInfo.Name] = tableData
		logger.Logger.Info("Exported table", zap.String("table", tableInfo.Name))
	}

	logger.Logger.Info("Data export completed")
	return exportData, nil
}

// exportTable exports all data from a specific table
func (m *Migrator) exportTable(tableInfo TableInfo) (any, error) {
	// Create a slice to hold all table data
	modelType := reflect.TypeOf(tableInfo.Model).Elem()
	sliceType := reflect.SliceOf(modelType)
	tableData := reflect.New(sliceType).Interface()

	// Fetch all data from the table
	if err := m.sourceConn.DB.Find(tableData).Error; err != nil {
		return nil, errors.Wrapf(err, "failed to fetch data from table")
	}

	return tableData, nil
}
