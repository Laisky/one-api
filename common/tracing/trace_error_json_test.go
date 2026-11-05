package tracing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// TestTraceFailureJSONFields checks actual encoded error events on synchronous
// trace failures, including unique contextual keys and the trace identifier.
func TestTraceFailureJSONFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "empty.db")), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	oldDB, oldMode, oldSinks := model.DB, config.TraceWriteMode, config.TraceSinks
	model.DB = db
	config.TraceWriteMode = config.TraceWriteModeSync
	config.TraceSinks = []string{config.TraceSinkDB}
	t.Cleanup(func() { model.DB = oldDB; config.TraceWriteMode = oldMode; config.TraceSinks = oldSinks })
	for _, tc := range []struct {
		name, field, value, message string
		run                         func(*gin.Context)
	}{
		{"timestamp", "timestamp_key", model.TimestampRequestCompleted, "failed to update trace timestamp", func(c *gin.Context) { RecordTraceTimestamp(c, model.TimestampRequestCompleted) }},
		{"status", "status", "503", "failed to update trace status", func(c *gin.Context) { RecordTraceStatus(c, 503) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "trace.json")
			lg, err := glog.New(glog.WithName("trace-json-test"), glog.WithLevel(glog.LevelDebug), glog.WithEncoding(glog.EncodingJSON), glog.WithOutputPaths([]string{path}), glog.WithErrorOutputPaths([]string{path}))
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			gmw.SetLogger(c, lg)
			tid := GetTraceID(c)
			require.NotEmpty(t, tid)
			tc.run(c)
			require.NoError(t, lg.Sync())
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			dec := json.NewDecoder(strings.NewReader(string(raw)))
			events := 0
			for {
				tok, err := dec.Token()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				require.Equal(t, json.Delim('{'), tok)
				counts := map[string]int{}
				values := map[string]any{}
				for dec.More() {
					key, err := dec.Token()
					require.NoError(t, err)
					name := key.(string)
					counts[name]++
					var v any
					require.NoError(t, dec.Decode(&v))
					values[name] = v
				}
				_, err = dec.Token()
				require.NoError(t, err)
				if values["message"] != tc.message {
					continue
				}
				events++
				require.Equal(t, 1, counts[tc.field])
				require.Equal(t, 1, counts["trace_id"])
				require.Equal(t, 1, counts["error"])
				require.Equal(t, tid, values["trace_id"])
				require.Equal(t, tc.value, fmt.Sprint(values[tc.field]))
				require.Contains(t, values["error"], "no such table")
			}
			require.Equal(t, 1, events)
		})
	}
}
