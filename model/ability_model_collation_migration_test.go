package model

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// TestMigrateAbilityModelCollationMySQL verifies that validated MySQL metadata
// produces a binary-collation ALTER TABLE statement.
func TestMigrateAbilityModelCollationMySQL(t *testing.T) {
	mock, closeDB := setupMySQLMockDB(t)

	metadataSQL := regexp.QuoteMeta(`SELECT COLUMN_TYPE AS column_type,
		CHARACTER_SET_NAME AS character_set_name,
		COLLATION_NAME AS collation_name,
		IS_NULLABLE AS is_nullable,
		COLUMN_DEFAULT AS column_default,
		EXTRA AS extra
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`)
	mock.ExpectQuery(metadataSQL).
		WithArgs("abilities", "model").
		WillReturnRows(sqlmock.NewRows([]string{
			"column_type", "character_set_name", "collation_name", "is_nullable", "column_default", "extra",
		}).AddRow("varchar(191)", "utf8mb4", "utf8mb4_0900_ai_ci", "NO", nil, ""))

	collationSQL := regexp.QuoteMeta(`SELECT COUNT(*) AS count FROM information_schema.collations
		WHERE character_set_name = ? AND collation_name = ?`)
	mock.ExpectQuery(collationSQL).
		WithArgs("utf8mb4", "utf8mb4_bin").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	alterSQL := regexp.QuoteMeta("ALTER TABLE `abilities` MODIFY COLUMN `model` varchar(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL")
	mock.ExpectExec(alterSQL).WillReturnResult(sqlmock.NewResult(0, 0))

	require.NoError(t, migrateAbilityModelCollationMySQL(DB))
	require.NoError(t, closeDB())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestMigrateAbilityModelCollationMySQLAlreadyBinary verifies that an already
// migrated column is left unchanged.
func TestMigrateAbilityModelCollationMySQLAlreadyBinary(t *testing.T) {
	mock, closeDB := setupMySQLMockDB(t)

	metadataSQL := regexp.QuoteMeta(`SELECT COLUMN_TYPE AS column_type,
		CHARACTER_SET_NAME AS character_set_name,
		COLLATION_NAME AS collation_name,
		IS_NULLABLE AS is_nullable,
		COLUMN_DEFAULT AS column_default,
		EXTRA AS extra
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`)
	mock.ExpectQuery(metadataSQL).
		WithArgs("abilities", "model").
		WillReturnRows(sqlmock.NewRows([]string{
			"column_type", "character_set_name", "collation_name", "is_nullable", "column_default", "extra",
		}).AddRow("varchar(191)", "utf8mb4", "utf8mb4_bin", "NO", nil, ""))

	require.NoError(t, migrateAbilityModelCollationMySQL(DB))
	require.NoError(t, closeDB())
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestValidateMySQLAbilityModelMigrationMetadata rejects metadata that would be
// unsafe to interpolate into migration DDL.
func TestValidateMySQLAbilityModelMigrationMetadata(t *testing.T) {
	t.Parallel()

	_, err := validateMySQLAbilityModelColumnType("varchar(191); DROP TABLE abilities")
	require.Error(t, err)
	_, err = validateMySQLAbilityModelColumnType("text")
	require.Error(t, err)

	require.Equal(t, "varchar(191)", mustValidateMySQLAbilityModelColumnType(t, "VARCHAR(191)"))
	require.Equal(t, "utf8mb4", mustValidateMySQLIdentifier(t, "utf8mb4", "character set"))
	_, err = validateMySQLIdentifier("utf8mb4`; DROP TABLE abilities", "character set")
	require.Error(t, err)
	_, err = validateMySQLAbilityModelNullability("UNKNOWN")
	require.Error(t, err)
}

// mustValidateMySQLAbilityModelColumnType validates a test column type and fails
// the current test when validation unexpectedly fails.
func mustValidateMySQLAbilityModelColumnType(t *testing.T, columnType string) string {
	t.Helper()
	value, err := validateMySQLAbilityModelColumnType(columnType)
	require.NoError(t, err)
	return value
}

// mustValidateMySQLIdentifier validates a test identifier and fails the current
// test when validation unexpectedly fails.
func mustValidateMySQLIdentifier(t *testing.T, identifier string, kind string) string {
	t.Helper()
	value, err := validateMySQLIdentifier(identifier, kind)
	require.NoError(t, err)
	return value
}
