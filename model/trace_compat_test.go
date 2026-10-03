package model

// Backward-compatibility regression tests for the Phase-1 trace storage change
// (proposal docs/proposals/20260905_observability-data-tiering.md, W1.5).
//
// The six ts_* columns are additive: a row written by this binary must remain
// fully readable by a binary that predates them, and a row written by such a
// binary must remain fully readable here. These tests pin both directions so a
// future change cannot quietly break rollback.
//
// The forward direction was additionally verified against the real pre-change
// tree (commit 397781e1) on PostgreSQL: that binary selected a row written by
// this one, parsed its timestamp document including external calls, and then
// inserted and updated rows of its own through the old synchronous path.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/logger"
)

// legacyTraceRow mirrors the Trace struct as it existed before the ts_* columns
// were added. Scanning a current row into it reproduces exactly what a
// pre-change binary sees.
type legacyTraceRow struct {
	Id         int    `gorm:"primaryKey;autoIncrement"`
	UUID       string `gorm:"type:char(36);column:uuid"`
	TraceId    string `gorm:"type:varchar(64);uniqueIndex;not null"`
	URL        string `gorm:"type:text;not null"`
	Method     string `gorm:"type:varchar(16);not null"`
	BodySize   int64  `gorm:"bigint;default:0"`
	Status     int    `gorm:"default:0"`
	Timestamps string `gorm:"type:text"`
	CreatedAt  int64  `gorm:"bigint;autoCreateTime:milli;index"`
	UpdatedAt  int64  `gorm:"bigint;autoUpdateTime:milli"`
}

// TableName binds the legacy shape to the traces table.
//
// Parameters: none.
//
// Return values:
//   - string: the physical table name.
func (legacyTraceRow) TableName() string { return "traces" }

// TestOldBinaryCanReadRowsWrittenByNewCode verifies a pre-change binary keeps
// working after this build has written to the traces table.
//
// It scans a row produced by the current write path into the pre-change struct
// shape: extra columns in the result set must be ignored rather than rejected,
// and the JSON timestamp document must still carry everything the old code
// depended on.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestOldBinaryCanReadRowsWrittenByNewCode(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-compat-newwrite-%")

	now := time.Now().UTC().UnixMilli()
	forwarded, completed := now+5, now+120

	row, _, err := NewTraceRow(TraceRowInput{
		TraceId:   "test-compat-newwrite-1",
		URL:       "/v1/chat/completions?model=gpt-4.1",
		Method:    "POST",
		BodySize:  2048,
		Status:    200,
		CreatedAt: now,
		Timestamps: &TraceTimestamps{
			RequestReceived:  &now,
			RequestForwarded: &forwarded,
			RequestCompleted: &completed,
			ExternalCalls:    []TraceExternalCall{{Source: "mcp", Tool: "web_search", DurationMs: 31}},
		},
	})
	require.NoError(t, err)

	written, err := InsertTraces(context.Background(), []*Trace{row}, 10)
	require.NoError(t, err)
	require.Equal(t, 1, written)

	var legacy legacyTraceRow
	require.NoError(t, DB.Where("trace_id = ?", "test-compat-newwrite-1").First(&legacy).Error,
		"a pre-change binary must be able to SELECT a row this build wrote")

	require.Equal(t, "POST", legacy.Method)
	require.Equal(t, 200, legacy.Status)
	require.Equal(t, int64(2048), legacy.BodySize)
	require.NotEmpty(t, legacy.UUID, "the server-assigned UUID must still be present")
	require.Equal(t, now, legacy.CreatedAt)

	// The JSON document is the only timestamp source a pre-change binary has, so
	// it must remain complete rather than being replaced by the new columns.
	var parsed TraceTimestamps
	require.NoError(t, json.Unmarshal([]byte(legacy.Timestamps), &parsed))
	require.NotNil(t, parsed.RequestReceived)
	require.NotNil(t, parsed.RequestForwarded)
	require.NotNil(t, parsed.RequestCompleted)
	require.Equal(t, completed, *parsed.RequestCompleted)
	require.Len(t, parsed.ExternalCalls, 1)
	require.Equal(t, "web_search", parsed.ExternalCalls[0].Tool)
}

// TestNewCodeReadsRowsWrittenByOldBinary verifies the reverse direction: a row
// carrying only the legacy JSON document, with every ts_* column NULL, still
// resolves to a complete timestamp view.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestNewCodeReadsRowsWrittenByOldBinary(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-compat-oldwrite-%")

	legacy := &legacyTraceRow{
		TraceId:    "test-compat-oldwrite-1",
		URL:        "/v1/chat/completions",
		Method:     "POST",
		BodySize:   1024,
		Status:     201,
		Timestamps: `{"request_received":10,"request_forwarded":20,"request_completed":90,"external_calls":[{"source":"mcp","tool":"fetch"}]}`,
		CreatedAt:  10,
	}
	require.NoError(t, DB.Create(legacy).Error)

	var current Trace
	require.NoError(t, DB.Where("trace_id = ?", "test-compat-oldwrite-1").First(&current).Error)

	require.Nil(t, current.TsRequestCompleted, "a row written by a pre-change binary has no ts_* values")

	ts, err := current.GetTraceTimestamps()
	require.NoError(t, err)
	require.NotNil(t, ts.RequestReceived)
	require.Equal(t, int64(10), *ts.RequestReceived)
	require.NotNil(t, ts.RequestCompleted)
	require.Equal(t, int64(90), *ts.RequestCompleted)
	require.Len(t, ts.ExternalCalls, 1)
	require.Equal(t, "fetch", ts.ExternalCalls[0].Tool)

	// The legacy synchronous path must still be able to update such a row.
	require.NoError(t, UpdateTraceTimestamp(nil, "test-compat-oldwrite-1", TimestampUpstreamCompleted))

	var updated Trace
	require.NoError(t, DB.Where("trace_id = ?", "test-compat-oldwrite-1").First(&updated).Error)
	after, err := updated.GetTraceTimestamps()
	require.NoError(t, err)
	require.NotNil(t, after.UpstreamCompleted)
	require.NotNil(t, after.RequestCompleted, "updating one mark must not drop the others")
	require.Len(t, after.ExternalCalls, 1, "updating one mark must not drop external calls")
}

// TestTraceWritesSucceedOnUnmigratedSchema verifies a node whose traces table
// predates the W1.5 columns can still write traces.
//
// Only the master runs AutoMigrate, so a NODE_TYPE=slave process upgraded ahead
// of its master sees exactly this schema. Naming the new columns there would
// fail every trace write for the whole rolling upgrade.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestTraceWritesSucceedOnUnmigratedSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)

	// Build the pre-change schema: no ts_* columns at all.
	require.NoError(t, db.AutoMigrate(&legacyTraceRow{}))
	require.False(t, db.Migrator().HasColumn(&Trace{}, "ts_request_received"),
		"the fixture must reproduce a table that predates the projection columns")

	prevDB := DB
	DB = db
	ResetTraceColumnProbe()
	t.Cleanup(func() { DB = prevDB; ResetTraceColumnProbe() })

	now := time.Now().UTC().UnixMilli()
	completed := now + 90
	row, _, err := NewTraceRow(TraceRowInput{
		TraceId:   "test-unmigrated-1",
		URL:       "/v1/chat/completions",
		Method:    "POST",
		Status:    200,
		CreatedAt: now,
		Timestamps: &TraceTimestamps{
			RequestReceived:  &now,
			RequestCompleted: &completed,
		},
	})
	require.NoError(t, err)

	written, err := InsertTraces(context.Background(), []*Trace{row}, 10)
	require.NoError(t, err, "a trace write must not fail merely because the schema is not migrated yet")
	require.Equal(t, 1, written)

	// The complete timestamp document must still be there: degrading drops the
	// projection columns, never the data.
	var stored legacyTraceRow
	require.NoError(t, db.Where("trace_id = ?", "test-unmigrated-1").First(&stored).Error)
	var parsed TraceTimestamps
	require.NoError(t, json.Unmarshal([]byte(stored.Timestamps), &parsed))
	require.NotNil(t, parsed.RequestReceived)
	require.NotNil(t, parsed.RequestCompleted)
	require.Equal(t, completed, *parsed.RequestCompleted)

	// The synchronous path must degrade the same way.
	ctx := gmw.SetLogger(context.Background(), logger.Logger)
	created, err := CreateTrace(ctx, "test-unmigrated-2", "/api/test", "GET", 0)
	require.NoError(t, err, "CreateTrace must not fail on an unmigrated schema")
	require.Equal(t, "test-unmigrated-2", created.TraceId)

	// The standalone profile uses the synchronous lifecycle by default, so
	// compatibility must cover mutations as well as the initial INSERT. Before
	// this regression test, UpdateTraceTimestamp always named the new projection
	// column and failed on an upgraded slave whose master had not migrated yet.
	require.NoError(t, UpdateTraceTimestamp(nil, created.TraceId, TimestampRequestCompleted),
		"the complete synchronous lifecycle must work on an unmigrated schema")

	var syncStored legacyTraceRow
	require.NoError(t, db.Where("trace_id = ?", created.TraceId).First(&syncStored).Error)
	var syncTimestamps TraceTimestamps
	require.NoError(t, json.Unmarshal([]byte(syncStored.Timestamps), &syncTimestamps))
	require.NotNil(t, syncTimestamps.RequestCompleted,
		"the legacy JSON document must retain synchronous lifecycle mutations")
}
