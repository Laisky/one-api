package model

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Live-database seeding and dialect-specific EXPLAIN helpers used to prove index usage.

func seedUUIDLiveUsers(t *testing.T, db *gorm.DB, prefix string, total int, missingPerPass int) {
	t.Helper()
	const chunk = 500
	for start := 1; start <= total; start += chunk {
		end := start + chunk - 1
		if end > total {
			end = total
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			var value any
			switch {
			case id <= missingPerPass:
				value = nil
			case id <= 2*missingPerPass:
				value = ""
			default:
				value = uuidMultiDBFixtureUUID(id)
			}
			rows = append(rows, map[string]any{
				"id":       id,
				"username": prefix + "-user-" + strconv.Itoa(id),
				"password": "password-hash",
				"uuid":     value,
			})
		}
		require.NoError(t, db.Table("users").Create(rows).Error, "seed users %d..%d", start, end)
	}
}

// seedUUIDLiveTokens inserts tokens whose (user_id, name) keys are unique and populated.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - total: number of tokens to insert.
//
// Return values: none.
func seedUUIDLiveTokens(t *testing.T, db *gorm.DB, total int) {
	t.Helper()
	const chunk = 500
	for start := 1; start <= total; start += chunk {
		end := start + chunk - 1
		if end > total {
			end = total
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			// Token names must repeat across users and stay unique within a user, which is
			// how real deployments look ("default", "prod") and is precisely the
			// distribution that makes the (user_id, name) composite index worth having.
			// Seeding globally unique names would make the single-column idx_tokens_name
			// equally selective, and the planner would then be free to pick either index —
			// proving nothing about composite-index usability.
			rows = append(rows, map[string]any{
				"id":      id,
				"user_id": ((id - 1) % uuidPlanTokenUsers) + 1,
				"key":     "plan-token-key-" + strconv.Itoa(id),
				"name":    "plan-token-" + strconv.Itoa((id-1)/uuidPlanTokenUsers),
				"uuid":    uuidMultiDBFixtureUUID(1000000 + id),
			})
		}
		require.NoError(t, db.Table("tokens").Create(rows).Error, "seed tokens %d..%d", start, end)
	}
}

// uuidAnalyzeTable refreshes planner statistics so EXPLAIN reflects the seeded distribution.
// Without it PostgreSQL guesses NULL selectivity and can pick a sequential scan for a plan that
// is genuinely index-driven in production.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - table: trusted table name.
//
// Return values: none.
func uuidAnalyzeTable(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	switch dialectName(db) {
	case "postgres":
		require.NoError(t, db.Exec("ANALYZE "+quoteIdentifier(db, table)).Error, "analyze %s", table)
	case "mysql":
		// ANALYZE TABLE returns a result set, so it must be drained rather than Exec'd.
		rows, err := db.Raw("ANALYZE TABLE " + quoteIdentifier(db, table)).Rows()
		require.NoError(t, err, "analyze %s", table)
		require.NoError(t, rows.Close())
	default:
		t.Fatalf("unsupported dialect %q", dialectName(db))
	}
}

// uuidExplainResult carries one EXPLAIN result set in a dialect-neutral form.
type uuidExplainResult struct {
	// columns are the plan columns in server order.
	columns []string
	// rows map each column to its value, with nil representing SQL NULL.
	rows []map[string]*string
}

// text renders the plan for t.Logf release evidence.
// Parameters: none.
//
// Return values:
//   - string: human-readable plan text.
func (result uuidExplainResult) text() string {
	lines := make([]string, 0, len(result.rows))
	for _, row := range result.rows {
		parts := make([]string, 0, len(result.columns))
		for _, column := range result.columns {
			value := "NULL"
			if row[column] != nil {
				value = *row[column]
			}
			if len(result.columns) == 1 {
				parts = append(parts, value)
				continue
			}
			parts = append(parts, column+"="+value)
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	return strings.Join(lines, "\n")
}

// uuidExplain runs EXPLAIN for one statement and returns every plan row.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - statement: statement to explain.
//
// Return values:
//   - uuidExplainResult: the dialect's plan rows.
func uuidExplain(t *testing.T, db *gorm.DB, statement string) uuidExplainResult {
	t.Helper()
	explain := "EXPLAIN " + statement
	if dialectName(db) == "postgres" {
		explain = "EXPLAIN (FORMAT TEXT) " + statement
	}

	rows, err := db.Raw(explain).Rows()
	require.NoError(t, err, "run %s", explain)
	defer func() { require.NoError(t, rows.Close()) }()

	columns, err := rows.Columns()
	require.NoError(t, err, "read plan columns")

	result := uuidExplainResult{columns: columns}
	for rows.Next() {
		holders := make([]any, len(columns))
		for i := range holders {
			holders[i] = &sql.NullString{}
		}
		require.NoError(t, rows.Scan(holders...), "scan plan row")
		record := make(map[string]*string, len(columns))
		for i, column := range columns {
			value := holders[i].(*sql.NullString)
			if !value.Valid {
				record[column] = nil
				continue
			}
			text := value.String
			record[column] = &text
		}
		result.rows = append(result.rows, record)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, result.rows, "%s returned no plan rows", explain)
	return result
}

// requireIndexedPlan asserts a statement is served by an index rather than a full scan, logs
// the plan as release evidence, and returns it for further dialect-specific assertions.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - statement: statement to explain.
//   - label: human-readable query name for evidence and failure messages.
//
// Return values:
//   - uuidExplainResult: the plan, already logged.
func requireIndexedPlan(t *testing.T, db *gorm.DB, statement string, label string) uuidExplainResult {
	t.Helper()
	result := uuidExplain(t, db, statement)
	t.Logf("EXPLAIN evidence [%s] %s\n%s\n%s", dialectName(db), label, statement, result.text())

	switch dialectName(db) {
	case "postgres":
		plan := result.text()
		require.NotContains(t, plan, "Seq Scan", "%s must not degenerate into a sequential scan", label)
		require.Contains(t, plan, "Index", "%s must be served by an index", label)
	case "mysql":
		key := result.rows[0]["key"]
		require.NotNil(t, key, "%s must choose an index, but EXPLAIN reported key=NULL", label)
		require.NotEmpty(t, *key, "%s must choose a named index", label)
	default:
		t.Fatalf("unsupported dialect %q", dialectName(db))
	}
	return result
}

// requirePlanUsesIndex asserts the planner actually chose one specific index.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - result: plan returned by requireIndexedPlan.
//   - index: index name the plan must use.
//   - label: human-readable query name for failure messages.
//
// Return values: none.
func requirePlanUsesIndex(t *testing.T, db *gorm.DB, result uuidExplainResult, index string, label string) {
	t.Helper()
	switch dialectName(db) {
	case "postgres":
		require.Contains(t, result.text(), index, "%s must be served by %s", label, index)
	case "mysql":
		key := result.rows[0]["key"]
		require.NotNil(t, key, "%s must be served by %s, but EXPLAIN reported key=NULL", label, index)
		require.Equal(t, index, *key, "%s must be served by %s", label, index)
	default:
		t.Fatalf("unsupported dialect %q", dialectName(db))
	}
}

// requirePlanConsidersIndex asserts one index is available to the planner for a query shape,
// which is what "the predicate is sargable against the UUID index" actually means.
// MySQL reports this directly as possible_keys. PostgreSQL has no equivalent column, so the
// chosen index is asserted instead, which is strictly stronger.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live handle.
//   - result: plan returned by requireIndexedPlan.
//   - index: index name the predicate must be able to use.
//   - label: human-readable query name for failure messages.
//
// Return values: none.
func requirePlanConsidersIndex(t *testing.T, db *gorm.DB, result uuidExplainResult, index string, label string) {
	t.Helper()
	switch dialectName(db) {
	case "postgres":
		require.Contains(t, result.text(), index, "%s must be served by %s", label, index)
	case "mysql":
		possible := result.rows[0]["possible_keys"]
		require.NotNil(t, possible, "%s must offer %s to the planner", label, index)
		require.Contains(t, *possible, index, "%s must offer %s to the planner", label, index)
	default:
		t.Fatalf("unsupported dialect %q", dialectName(db))
	}
}

// uuidPrimaryKeyIndexName returns the dialect's name for a table's primary key index.
// Parameters:
//   - db: live handle.
//   - table: trusted table name.
//
// Return values:
//   - string: primary key index name as it appears in EXPLAIN output.
func uuidPrimaryKeyIndexName(db *gorm.DB, table string) string {
	if dialectName(db) == "mysql" {
		return "PRIMARY"
	}
	return table + "_pkey"
}
