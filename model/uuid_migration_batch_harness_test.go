package model

import (
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Bind recorders and seeding helpers for the bind-budget acceptance tests.

const uuidBatchLogUserID = 1

// uuidBatchBindRecorder records the bind count of every observed statement so tests can
// assert the real generated SQL — not only the chunking arithmetic — stays inside the
// portable bind budget.
type uuidBatchBindRecorder struct {
	mu    sync.Mutex
	binds []int
}

// record appends one observed bind count.
// Parameters:
//   - count: number of bound variables in the observed statement.
//
// Return values: none.
func (recorder *uuidBatchBindRecorder) record(count int) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.binds = append(recorder.binds, count)
}

// observations returns a copy of every observed bind count in execution order.
// Parameters: none.
//
// Return values:
//   - []int: bind counts of the observed statements.
func (recorder *uuidBatchBindRecorder) observations() []int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	observed := make([]int, len(recorder.binds))
	copy(observed, recorder.binds)
	return observed
}

// reset drops every observation so one recorder can serve consecutive subtests.
// Parameters: none.
//
// Return values: none.
func (recorder *uuidBatchBindRecorder) reset() {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.binds = nil
}

// max returns the largest observed bind count, or zero when nothing was observed.
// Parameters: none.
//
// Return values:
//   - int: maximum observed bind count.
func (recorder *uuidBatchBindRecorder) max() int {
	largest := 0
	for _, count := range recorder.observations() {
		if count > largest {
			largest = count
		}
	}
	return largest
}

// installUUIDBatchUpdateBindRecorder records the bind count of every UPDATE the handle runs.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle to instrument.
//
// Return values:
//   - *uuidBatchBindRecorder: recorder observing update statements.
func installUUIDBatchUpdateBindRecorder(t *testing.T, db *gorm.DB) *uuidBatchBindRecorder {
	t.Helper()
	recorder := &uuidBatchBindRecorder{}
	err := db.Callback().Update().After("gorm:update").Register("uuidbatchtest:bind_update", func(tx *gorm.DB) {
		recorder.record(len(tx.Statement.Vars))
	})
	require.NoError(t, err)
	return recorder
}

// installUUIDBatchQueryBindRecorder records the bind count of every SELECT against one table.
// Parameters:
//   - t: test handle used for assertions.
//   - db: database handle to instrument.
//   - table: table whose queries are observed.
//
// Return values:
//   - *uuidBatchBindRecorder: recorder observing reference queries.
func installUUIDBatchQueryBindRecorder(t *testing.T, db *gorm.DB, table string) *uuidBatchBindRecorder {
	t.Helper()
	recorder := &uuidBatchBindRecorder{}
	err := db.Callback().Query().After("gorm:query").Register("uuidbatchtest:bind_query", func(tx *gorm.DB) {
		if tx.Statement.Table != table {
			return
		}
		recorder.record(len(tx.Statement.Vars))
	})
	require.NoError(t, err)
	return recorder
}

// uuidBatchTestUUID builds a deterministic canonical UUID for one seeded row id.
// Parameters:
//   - id: row id.
//
// Return values:
//   - string: canonical hyphenated UUID unique to the id.
func uuidBatchTestUUID(id int) string {
	return fmt.Sprintf("018f0000-0000-7000-8000-%012d", id)
}

// uuidBatchTokenName builds the deterministic token name observed for one seeded log row.
// Parameters:
//   - id: log row id.
//
// Return values:
//   - string: token name unique to the log row.
func uuidBatchTokenName(id int) string {
	return "batch-token-" + strconv.Itoa(id)
}

// seedUUIDBatchUsers inserts count legacy users that carry an inviter reference but no UUIDs.
// The rows feed both the zero-condition (users.uuid) and one-condition (users.inviter_uuid)
// conditional-update shapes.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - count: number of users to insert, with ids 1..count.
//
// Return values: none.
func seedUUIDBatchUsers(t *testing.T, db *gorm.DB, count int) {
	t.Helper()
	seedUUIDBatchUserRows(t, db, count, false)
}

// seedUUIDBatchUsersWithUUID inserts count users that already carry deterministic owned UUIDs.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - count: number of users to insert, with ids 1..count.
//
// Return values: none.
func seedUUIDBatchUsersWithUUID(t *testing.T, db *gorm.DB, count int) {
	t.Helper()
	seedUUIDBatchUserRows(t, db, count, true)
}

// seedUUIDBatchUserRows inserts count users in bounded inserts.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - count: number of users to insert, with ids 1..count.
//   - withUUID: whether each row carries a populated owned UUID.
//
// Return values: none.
func seedUUIDBatchUserRows(t *testing.T, db *gorm.DB, count int, withUUID bool) {
	t.Helper()
	const chunk = 200
	for start := 1; start <= count; start += chunk {
		end := start + chunk - 1
		if end > count {
			end = count
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			row := map[string]any{
				"id":         id,
				"username":   "batch-user-" + strconv.Itoa(id),
				"password":   "password-hash",
				"inviter_id": id,
			}
			if withUUID {
				row["uuid"] = uuidBatchTestUUID(id)
			}
			rows = append(rows, row)
		}
		require.NoError(t, db.Table("users").Create(rows).Error)
	}
}

// seedUUIDBatchLogs inserts count legacy log rows with observable user and token-name fields.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - count: number of logs to insert, with ids 1..count.
//
// Return values: none.
func seedUUIDBatchLogs(t *testing.T, db *gorm.DB, count int) {
	t.Helper()
	const chunk = 200
	for start := 1; start <= count; start += chunk {
		end := start + chunk - 1
		if end > count {
			end = count
		}
		rows := make([]map[string]any, 0, end-start+1)
		for id := start; id <= end; id++ {
			rows = append(rows, map[string]any{
				"id":         id,
				"user_id":    uuidBatchLogUserID,
				"channel_id": 1,
				"type":       1,
				"token_name": uuidBatchTokenName(id),
				"content":    "batch log",
			})
		}
		require.NoError(t, db.Table("logs").Create(rows).Error)
	}
}

// seedUUIDBatchTokens inserts count tokens that all share one (user_id, name) composite key.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle receiving the rows.
//   - userID: owning user id shared by every inserted token.
//   - name: token name shared by every inserted token.
//   - firstID: first token id; ids run firstID..firstID+count-1.
//   - count: number of tokens to insert.
//   - uuidForID: owned UUID for one token id; an empty result seeds an unpopulated UUID.
//
// Return values: none.
func seedUUIDBatchTokens(t *testing.T, db *gorm.DB, userID int, name string, firstID int, count int, uuidForID func(id int) string) {
	t.Helper()
	const chunk = 200
	for start := 0; start < count; start += chunk {
		end := start + chunk
		if end > count {
			end = count
		}
		rows := make([]map[string]any, 0, end-start)
		for offset := start; offset < end; offset++ {
			id := firstID + offset
			rows = append(rows, map[string]any{
				"id":      id,
				"user_id": userID,
				"key":     fmt.Sprintf("batch-key-%08d", id),
				"name":    name,
				"uuid":    uuidForID(id),
			})
		}
		require.NoError(t, db.Table("tokens").Create(rows).Error)
	}
}

// uuidBatchOwnedValues builds total zero-condition owned-UUID update values.
// Parameters:
//   - total: number of rows to update, with ids 1..total.
//
// Return values:
//   - []uuidConditionalValue: owned-UUID update values.
func uuidBatchOwnedValues(total int) []uuidConditionalValue {
	values := make([]uuidConditionalValue, 0, total)
	for id := 1; id <= total; id++ {
		values = append(values, uuidConditionalValue{id: id, value: uuidBatchTestUUID(id)})
	}
	return values
}

// uuidBatchInviterValues builds total one-condition (inviter_id) FK update values.
// Parameters:
//   - total: number of rows to update, with ids 1..total.
//
// Return values:
//   - []uuidConditionalValue: one-condition update values.
func uuidBatchInviterValues(total int) []uuidConditionalValue {
	values := make([]uuidConditionalValue, 0, total)
	for id := 1; id <= total; id++ {
		values = append(values, uuidConditionalValue{
			id:         id,
			conditions: []uuidColumnValue{{column: "inviter_id", value: id}},
			value:      uuidBatchTestUUID(id),
		})
	}
	return values
}

// uuidBatchTokenNameValues builds total two-condition (user_id, token_name) update values.
// Parameters:
//   - total: number of rows to update, with ids 1..total.
//
// Return values:
//   - []uuidConditionalValue: two-condition update values.
func uuidBatchTokenNameValues(total int) []uuidConditionalValue {
	values := make([]uuidConditionalValue, 0, total)
	for id := 1; id <= total; id++ {
		values = append(values, uuidConditionalValue{
			id: id,
			conditions: []uuidColumnValue{
				{column: "user_id", value: uuidBatchLogUserID},
				{column: "token_name", value: uuidBatchTokenName(id)},
			},
			value: uuidBatchTestUUID(id),
		})
	}
	return values
}
