package model

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNewTraceRowSanitizesAndBoundsURL verifies the shared row builder applies
// the same sanitization and length bound as the synchronous create path.
func TestNewTraceRowSanitizesAndBoundsURL(t *testing.T) {
	t.Run("redacts sensitive query values", func(t *testing.T) {
		row, truncated, err := NewTraceRow(TraceRowInput{
			TraceId: "test-trace-row-redaction",
			URL:     "/api/user/register?turnstile=secret-token&email=user@example.com",
			Method:  "POST",
		})
		require.NoError(t, err)
		require.False(t, truncated)
		require.NotContains(t, row.URL, "secret-token")
		require.Contains(t, row.URL, "turnstile=%5Bredacted%5D")
		require.Contains(t, row.URL, "email=user%40example.com")
	})

	t.Run("bounds oversized URLs", func(t *testing.T) {
		longURL := "/api/verification?payload=" + strings.Repeat("abc123", 1000)
		require.Greater(t, len(longURL), maxTraceURLLength)

		row, truncated, err := NewTraceRow(TraceRowInput{
			TraceId: "test-trace-row-builder",
			URL:     longURL,
			Method:  "GET",
		})
		require.NoError(t, err)
		require.True(t, truncated)
		require.Equal(t, maxTraceURLLength, len(row.URL))
	})
}

// TestNewTraceRowProjectsTimestampColumns verifies the timestamp document is
// written to both the JSON column and the dedicated per-timestamp columns.
func TestNewTraceRowProjectsTimestampColumns(t *testing.T) {
	received := int64(1000)
	forwarded := int64(1100)
	completed := int64(1500)

	row, _, err := NewTraceRow(TraceRowInput{
		TraceId:   "test-trace-row-columns",
		URL:       "/api/test",
		Method:    "POST",
		Status:    201,
		CreatedAt: received,
		Timestamps: &TraceTimestamps{
			RequestReceived:  &received,
			RequestForwarded: &forwarded,
			RequestCompleted: &completed,
		},
	})
	require.NoError(t, err)

	require.NotNil(t, row.TsRequestReceived)
	require.Equal(t, received, *row.TsRequestReceived)
	require.NotNil(t, row.TsRequestForwarded)
	require.Equal(t, forwarded, *row.TsRequestForwarded)
	require.NotNil(t, row.TsRequestCompleted)
	require.Equal(t, completed, *row.TsRequestCompleted)
	require.Nil(t, row.TsUpstreamCompleted)
	require.Equal(t, received, row.CreatedAt)
	require.Equal(t, 201, row.Status)

	// The JSON document is written too, so a pre-migration binary reading this
	// row still sees a complete timestamp document.
	var parsed TraceTimestamps
	require.NoError(t, json.Unmarshal([]byte(row.Timestamps), &parsed))
	require.NotNil(t, parsed.RequestForwarded)
	require.Equal(t, forwarded, *parsed.RequestForwarded)
}

// TestGetTraceTimestampsOverlaysColumns verifies the read path merges the
// dedicated columns over the JSON document, and still works for legacy rows
// that only carry the document.
func TestGetTraceTimestampsOverlaysColumns(t *testing.T) {
	t.Run("columns win over document", func(t *testing.T) {
		fromColumn := int64(4242)
		row := &Trace{
			TraceId:            "test-overlay-columns",
			Timestamps:         `{"request_received": 1, "request_completed": 2}`,
			TsRequestCompleted: &fromColumn,
		}
		ts, err := row.GetTraceTimestamps()
		require.NoError(t, err)
		require.NotNil(t, ts.RequestReceived)
		require.Equal(t, int64(1), *ts.RequestReceived)
		require.NotNil(t, ts.RequestCompleted)
		require.Equal(t, fromColumn, *ts.RequestCompleted)
	})

	t.Run("legacy document only", func(t *testing.T) {
		row := &Trace{
			TraceId:    "test-overlay-legacy",
			Timestamps: `{"request_received": 7, "external_calls":[{"source":"mcp"}]}`,
		}
		ts, err := row.GetTraceTimestamps()
		require.NoError(t, err)
		require.NotNil(t, ts.RequestReceived)
		require.Equal(t, int64(7), *ts.RequestReceived)
		require.Len(t, ts.ExternalCalls, 1)
	})

	t.Run("columns only", func(t *testing.T) {
		received := int64(11)
		row := &Trace{
			TraceId:           "test-overlay-columns-only",
			Timestamps:        "",
			TsRequestReceived: &received,
		}
		ts, err := row.GetTraceTimestamps()
		require.NoError(t, err)
		require.NotNil(t, ts.RequestReceived)
		require.Equal(t, received, *ts.RequestReceived)
	})

	t.Run("malformed document is reported", func(t *testing.T) {
		row := &Trace{TraceId: "test-overlay-broken", Timestamps: "{not json"}
		_, err := row.GetTraceTimestamps()
		require.Error(t, err)
	})
}

// TestInsertTracesWritesEveryRow verifies multi-row inserts persist the whole
// batch across several chunks.
func TestInsertTracesWritesEveryRow(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-insert-batch-%")

	// Use a current timestamp: these rows live in the shared test database, and
	// a 1970 created_at would make them expired input for the retention sweeper
	// running in another test.
	now := time.Now().UTC().UnixMilli()

	rows := make([]*Trace, 0, 5)
	for i := range 5 {
		row, _, err := NewTraceRow(TraceRowInput{
			TraceId:   "test-insert-batch-" + string(rune('a'+i)),
			URL:       "/api/test",
			Method:    "GET",
			Status:    200,
			CreatedAt: now + int64(i),
		})
		require.NoError(t, err)
		rows = append(rows, row)
	}

	written, err := InsertTraces(context.Background(), rows, 2)
	require.NoError(t, err)
	require.Equal(t, 5, written)

	var count int64
	require.NoError(t, DB.Model(&Trace{}).Where("trace_id LIKE 'test-insert-batch-%'").Count(&count).Error)
	require.Equal(t, int64(5), count)

	// Every row must carry a server-assigned UUID: batch inserts still run the
	// BeforeCreate hook.
	var stored []Trace
	require.NoError(t, DB.Where("trace_id LIKE 'test-insert-batch-%'").Find(&stored).Error)
	require.Len(t, stored, 5)
	for _, s := range stored {
		require.NotEmpty(t, s.UUID)
	}
}

// TestInsertTracesSurvivesDuplicateTraceID verifies one duplicate row inside a
// batch cannot discard the rest of that batch.
func TestInsertTracesSurvivesDuplicateTraceID(t *testing.T) {
	setupTestDatabase(t)
	cleanupTraces(t, "test-insert-dup-%")

	existing, _, err := NewTraceRow(TraceRowInput{
		TraceId: "test-insert-dup-existing", URL: "/api/test", Method: "GET",
	})
	require.NoError(t, err)
	_, err = InsertTraces(context.Background(), []*Trace{existing}, 10)
	require.NoError(t, err)

	duplicate, _, err := NewTraceRow(TraceRowInput{
		TraceId: "test-insert-dup-existing", URL: "/api/test", Method: "GET",
	})
	require.NoError(t, err)
	fresh, _, err := NewTraceRow(TraceRowInput{
		TraceId: "test-insert-dup-fresh", URL: "/api/test", Method: "GET",
	})
	require.NoError(t, err)

	written, err := InsertTraces(context.Background(), []*Trace{duplicate, fresh}, 10)
	require.NoError(t, err, "a duplicate trace id must not surface as a batch failure")
	require.Equal(t, 1, written)

	var count int64
	require.NoError(t, DB.Model(&Trace{}).Where("trace_id = ?", "test-insert-dup-fresh").Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// TestInsertTracesEmptyBatch verifies the no-op contract.
func TestInsertTracesEmptyBatch(t *testing.T) {
	written, err := InsertTraces(context.Background(), nil, 10)
	require.NoError(t, err)
	require.Zero(t, written)
}

// TestIsDuplicateTraceKeyError verifies driver-text detection, which is what
// actually fires because gorm error translation is disabled in this project.
func TestIsDuplicateTraceKeyError(t *testing.T) {
	require.False(t, IsDuplicateTraceKeyError(nil))
	require.True(t, IsDuplicateTraceKeyError(errString("UNIQUE constraint failed: traces.trace_id")))
	require.True(t, IsDuplicateTraceKeyError(errString("Error 1062: Duplicate entry")))
	require.True(t, IsDuplicateTraceKeyError(errString("ERROR: duplicate key value violates unique constraint (SQLSTATE 23505)")))
	require.False(t, IsDuplicateTraceKeyError(errString("connection refused")))
}

// errString is a minimal error type for table-driven error classification tests.
type errString string

// Error implements the error interface.
func (e errString) Error() string { return string(e) }

// cleanupTraces removes rows matching a trace-id prefix pattern before and after
// a test. The model test suite shares one database file, so a test that leaves
// trace rows behind changes what a later retention test observes.
//
// Parameters:
//   - t: the test, used to register cleanup and to fail fast.
//   - pattern: a SQL LIKE pattern matched against trace_id.
//
// Return values: none.
func cleanupTraces(t *testing.T, pattern string) {
	t.Helper()
	remove := func() {
		require.NoError(t, DB.Exec("DELETE FROM traces WHERE trace_id LIKE ?", pattern).Error)
	}
	remove()
	t.Cleanup(remove)
}
