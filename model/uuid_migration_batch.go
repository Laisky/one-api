package model

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

const (
	// uuidReferenceBatchSize bounds how many distinct references one lookup resolves.
	uuidReferenceBatchSize = 400
	// uuidUpdateRowLimit is the configured conditional-update row limit. The effective
	// chunk is the smaller of this limit and the bind budget.
	uuidUpdateRowLimit = 200
	// uuidBindBudget is the conservative portable bind ceiling for one statement. It sits
	// below SQLite's 999-variable default so every dialect accepts the generated SQL.
	uuidBindBudget = 900
)

// uuidConditionalValue is one conditional update: write value into the target column of
// row id, but only while every observed condition still holds.
type uuidConditionalValue struct {
	id         int
	conditions []uuidColumnValue
	value      string
}

// uuidColumnValue is one observed column value rechecked by a conditional update.
type uuidColumnValue struct {
	column string
	value  any
}

// uuidTokenNameKey is one deduplicated (user_id, token_name) composite reference.
type uuidTokenNameKey struct {
	userID int
	name   string
}

// uuidConditionalBindsPerRow returns the binds one conditional-update row consumes.
// The generated statement spends (conditions + 2) binds in the CASE arm — one per
// rechecked column, one for the id, one for the written value — and (conditions + 1)
// binds in the WHERE list, giving three binds per owned-UUID row, five per one-FK row,
// and seven per logs.token_uuid row.
// Parameters:
//   - conditions: number of observed columns rechecked per row.
//
// Return values:
//   - int: binds consumed by one row.
func uuidConditionalBindsPerRow(conditions int) int {
	return 2*conditions + 3
}

// uuidChunkSize derives a bind-safe chunk size instead of assuming a fixed row count.
// Parameters:
//   - configuredLimit: configured maximum rows or keys per statement.
//   - bindsPerRow: binds consumed by one row or key.
//   - fixedBinds: binds consumed by arguments outside the per-row list.
//
// Return values:
//   - int: rows per statement, at least one and never exceeding the bind budget.
func uuidChunkSize(configuredLimit int, bindsPerRow int, fixedBinds int) int {
	if bindsPerRow <= 0 {
		return configuredLimit
	}
	available := uuidBindBudget - fixedBinds
	size := available / bindsPerRow
	if size > configuredLimit {
		size = configuredLimit
	}
	if size < 1 {
		size = 1
	}
	return size
}

// loadIDUUIDMapForIDs returns bounded live row id-to-UUID mappings for the supplied ids.
// Only references with populated owner UUIDs are returned, so an orphan or a not-yet-filled
// owner simply yields no update for that row.
// Parameters:
//   - ctx: context controlling the database read.
//   - db: database handle authoritatively owning the reference table.
//   - table: trusted reference table name.
//   - ids: reference ids observed in the current target batch.
//
// Return values:
//   - map[int]string: internal id to external UUID for resolved references.
//   - error: wrapped database error when a bounded reference query fails.
func loadIDUUIDMapForIDs(ctx context.Context, db *gorm.DB, table string, ids []int) (map[int]string, error) {
	uniqueIDs := uniquePositiveInts(ids)
	refs := make(map[int]string, len(uniqueIDs))
	chunk := uuidChunkSize(uuidReferenceBatchSize, 1, 0)
	for start := 0; start < len(uniqueIDs); start += chunk {
		end := start + chunk
		if end > len(uniqueIDs) {
			end = len(uniqueIDs)
		}
		rows := []struct {
			Id   int    `gorm:"column:id"`
			UUID string `gorm:"column:uuid"`
		}{}
		err := db.WithContext(ctx).
			Table(table).
			Select("id, uuid").
			Where(quoteIdentifier(db, "id")+" IN ? AND "+quoteIdentifier(db, "uuid")+" IS NOT NULL AND "+quoteIdentifier(db, "uuid")+" != ''", uniqueIDs[start:end]).
			Find(&rows).Error
		if err != nil {
			return nil, errors.Wrapf(err, "load bounded uuid map for %s", table)
		}
		for _, row := range rows {
			refs[row.Id] = row.UUID
		}
	}
	return refs, nil
}

// resolveTokenUUIDsForLogRows returns unique token UUIDs for the current log batch only.
// Parameters:
//   - ctx: context controlling the database read.
//   - tokenDB: database handle authoritatively owning tokens.
//   - rows: log rows with observed user ids and token names.
//
// Return values:
//   - map[string]string: composite key to token UUID for unambiguous references.
//   - int: number of ambiguous token-name references observed in the batch.
//   - error: wrapped database error when a bounded token query fails.
func resolveTokenUUIDsForLogRows(ctx context.Context, tokenDB *gorm.DB, rows []uuidLogTokenRow) (map[string]string, int, error) {
	keys := uniqueTokenNameKeys(rows)
	refs := make(map[string]string, len(keys))
	ambiguous := 0
	chunk := uuidChunkSize(uuidReferenceBatchSize, 2, 0)
	for start := 0; start < len(keys); start += chunk {
		end := start + chunk
		if end > len(keys) {
			end = len(keys)
		}
		chunkRefs, chunkAmbiguous, err := resolveTokenUUIDsForKeys(ctx, tokenDB, keys[start:end])
		if err != nil {
			return nil, 0, err
		}
		for key, uuid := range chunkRefs {
			refs[key] = uuid
		}
		ambiguous += chunkAmbiguous
	}
	return refs, ambiguous, nil
}

// resolveTokenUUIDsForKeys returns unambiguous token UUIDs for bounded user/name keys.
// The lookup is a batch-local aggregate grouped by (user_id, name), so it returns at most
// one row per requested key regardless of how many duplicate token rows share that key.
// A key matched by 100,000 duplicate tokens therefore materializes exactly one row. A UUID
// is emitted only when exactly one populated token row matches.
// Parameters:
//   - ctx: context controlling the database read.
//   - tokenDB: database handle authoritatively owning tokens.
//   - keys: deduplicated user-token keys from a single bounded chunk.
//
// Return values:
//   - map[string]string: composite key to token UUID for unambiguous references.
//   - int: number of ambiguous keys matching more than one populated token.
//   - error: wrapped database error when the bounded token query fails.
func resolveTokenUUIDsForKeys(ctx context.Context, tokenDB *gorm.DB, keys []uuidTokenNameKey) (map[string]string, int, error) {
	refs := map[string]string{}
	if len(keys) == 0 {
		return refs, 0, nil
	}

	userIDColumn := quoteIdentifier(tokenDB, "user_id")
	nameColumn := quoteIdentifier(tokenDB, "name")
	uuidColumn := quoteIdentifier(tokenDB, "uuid")

	predicate := strings.Builder{}
	args := make([]any, 0, len(keys)*2)
	for i, key := range keys {
		if i > 0 {
			predicate.WriteString(" OR ")
		}
		predicate.WriteString("(")
		predicate.WriteString(userIDColumn)
		predicate.WriteString(" = ? AND ")
		predicate.WriteString(nameColumn)
		predicate.WriteString(" = ?)")
		args = append(args, key.userID, key.name)
	}

	rows := []uuidTokenNameRow{}
	sql := "SELECT " + userIDColumn + ", " + nameColumn +
		", MIN(" + uuidColumn + ") AS " + quoteIdentifier(tokenDB, "uuid") +
		", COUNT(*) AS " + quoteIdentifier(tokenDB, "total") +
		" FROM " + quoteIdentifier(tokenDB, "tokens") +
		" WHERE " + uuidColumn + " IS NOT NULL AND " + uuidColumn + " != ''" +
		" AND " + nameColumn + " != '' AND (" + predicate.String() + ")" +
		" GROUP BY " + userIDColumn + ", " + nameColumn
	if err := tokenDB.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, 0, errors.Wrap(err, "load bounded token uuid references")
	}

	ambiguous := 0
	for _, row := range rows {
		if row.Total != 1 {
			// An ambiguous historical token name is reported and left unresolved; the
			// migration never chooses among duplicate matches.
			ambiguous++
			continue
		}
		refs[userTokenNameKey(row.UserID, row.Name)] = row.UUID
	}
	return refs, ambiguous, nil
}

// applyConditionalStringColumnRows updates one string column while rechecking observed fields.
// The write predicate always requires a still-missing UUID plus every observed reference
// column, so a concurrent change produces zero affected rows and is retried from the new
// state on a later pass rather than writing a stale value.
// Parameters:
//   - ctx: context controlling the database write.
//   - db: database handle containing the target table.
//   - table: trusted target table name.
//   - column: trusted target string column name.
//   - values: row values with observed reference fields to recheck.
//
// Return values:
//   - int: number of rows affected by the conditional update statements.
//   - error: wrapped database error when an update fails.
func applyConditionalStringColumnRows(ctx context.Context, db *gorm.DB, table string, column string, values []uuidConditionalValue) (int, error) {
	if len(values) == 0 {
		return 0, nil
	}
	sort.Slice(values, func(i int, j int) bool {
		return values[i].id < values[j].id
	})

	conditions := 0
	for _, value := range values {
		if len(value.conditions) > conditions {
			conditions = len(value.conditions)
		}
	}
	chunk := uuidChunkSize(uuidUpdateRowLimit, uuidConditionalBindsPerRow(conditions), 0)

	total := 0
	for start := 0; start < len(values); start += chunk {
		end := start + chunk
		if end > len(values) {
			end = len(values)
		}
		updated, err := applyConditionalStringColumnChunk(ctx, db, table, column, values[start:end])
		if err != nil {
			return total, err
		}
		total += updated
	}
	return total, nil
}

// applyConditionalStringColumnChunk updates one bind-safe chunk with a conditional CASE.
// Parameters:
//   - ctx: context controlling the database write.
//   - db: database handle containing the target table.
//   - table: trusted target table name.
//   - column: trusted target string column name.
//   - values: bind-safe chunk of row values with observed reference fields.
//
// Return values:
//   - int: number of rows affected by the chunk update.
//   - error: wrapped database error when the update fails.
func applyConditionalStringColumnChunk(ctx context.Context, db *gorm.DB, table string, column string, values []uuidConditionalValue) (int, error) {
	caseSQL := strings.Builder{}
	caseSQL.WriteString("CASE")
	caseArgs := make([]any, 0, len(values)*4)
	whereSQL := strings.Builder{}
	whereArgs := make([]any, 0, len(values)*3)

	for i, value := range values {
		if i > 0 {
			whereSQL.WriteString(" OR ")
		}
		conditionSQL, conditionArgs := conditionalValuePredicate(db, value)
		caseSQL.WriteString(" WHEN ")
		caseSQL.WriteString(conditionSQL)
		caseSQL.WriteString(" THEN ?")
		caseArgs = append(caseArgs, conditionArgs...)
		caseArgs = append(caseArgs, value.value)

		whereSQL.WriteString("(")
		whereSQL.WriteString(conditionSQL)
		whereSQL.WriteString(")")
		whereArgs = append(whereArgs, conditionArgs...)
	}
	caseSQL.WriteString(" ELSE ")
	caseSQL.WriteString(quoteIdentifier(db, column))
	caseSQL.WriteString(" END")

	missingSQL := "(" + quoteIdentifier(db, column) + " IS NULL OR " + quoteIdentifier(db, column) + " = '')"
	var affected int64
	err := runWithSQLiteBusyRetry(ctx, func() error {
		result := db.WithContext(ctx).
			Table(table).
			Where(missingSQL+" AND ("+whereSQL.String()+")", whereArgs...).
			Update(column, gorm.Expr(caseSQL.String(), caseArgs...))
		affected = result.RowsAffected
		return result.Error
	})
	if err != nil {
		return int(affected), errors.Wrapf(err, "conditionally update %s.%s", table, column)
	}
	return int(affected), nil
}

// conditionalValuePredicate returns the SQL predicate and args for one observed row.
// Parameters:
//   - db: database handle whose dialect controls identifier quoting.
//   - value: row id and observed reference fields to recheck.
//
// Return values:
//   - string: SQL predicate fragment for the row.
//   - []any: bound arguments for the predicate.
func conditionalValuePredicate(db *gorm.DB, value uuidConditionalValue) (string, []any) {
	predicate := strings.Builder{}
	args := make([]any, 0, len(value.conditions)+1)
	predicate.WriteString(quoteIdentifier(db, "id"))
	predicate.WriteString(" = ?")
	args = append(args, value.id)
	for _, condition := range value.conditions {
		predicate.WriteString(" AND ")
		predicate.WriteString(quoteIdentifier(db, condition.column))
		predicate.WriteString(" = ?")
		args = append(args, condition.value)
	}
	return predicate.String(), args
}

// uniquePositiveInts deduplicates and sorts positive integer ids.
// Parameters:
//   - ids: possibly repeated ids from a target batch.
//
// Return values:
//   - []int: sorted positive ids with duplicates removed.
func uniquePositiveInts(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	unique := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	sort.Ints(unique)
	return unique
}

// uniqueTokenNameKeys deduplicates and sorts log token-name references.
// Parameters:
//   - rows: log rows from a target batch.
//
// Return values:
//   - []uuidTokenNameKey: sorted positive user/token-name keys.
func uniqueTokenNameKeys(rows []uuidLogTokenRow) []uuidTokenNameKey {
	seen := make(map[uuidTokenNameKey]struct{}, len(rows))
	keys := make([]uuidTokenNameKey, 0, len(rows))
	for _, row := range rows {
		if row.UserID <= 0 || row.TokenName == "" {
			continue
		}
		key := uuidTokenNameKey{userID: row.UserID, name: row.TokenName}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		if keys[i].userID == keys[j].userID {
			return keys[i].name < keys[j].name
		}
		return keys[i].userID < keys[j].userID
	})
	return keys
}

// userTokenNameKey builds a stable map key for user-scoped token names.
// The NUL separator cannot appear in a token name, so the key is unambiguous.
// Parameters:
//   - userID: internal user id.
//   - tokenName: token display name.
//
// Return values:
//   - string: composite map key.
func userTokenNameKey(userID int, tokenName string) string {
	return strconv.Itoa(userID) + "\x00" + tokenName
}
