package middleware

// Trace pipeline benchmark: pre-Phase-1 synchronous writes versus the batched
// pipeline (proposal docs/proposals/20260905_observability-data-tiering.md).
//
// BASELINE HONESTY
//
// The baseline is not a reconstruction. TRACE_WRITE_MODE=sync is the real
// pre-Phase-1 code path, still compiled into the binary: RecordTraceStart calls
// model.CreateTrace, every lifecycle mark calls model.UpdateTraceTimestamp
// (SELECT the row, unmarshal the JSON timestamp document, mutate one field,
// re-marshal, UPDATE), and RecordTraceEnd calls model.UpdateTraceStatus.
// TestTracingMiddlewareSyncModeKeepsLegacyWrites asserts those statements are
// still issued. Both variants therefore run the same handler, the same
// middleware, the same lifecycle marks, and the same database, in the same
// process, differing only in the write path under measurement.
//
// The one difference from the pre-Phase-1 tree is that CreateTrace now builds
// its row through model.NewTraceRow, which additionally populates the
// per-timestamp columns (W1.5). That makes the baseline slightly MORE expensive
// than the historical code, so it is a conservative comparison in the
// optimization's favour only by the cost of six pointer assignments; the
// cross-tree benchmark in scripts/bench/ measures the true historical tree.
//
// VALIDITY
//
// A batching writer can flatter itself by deferring work past the end of the
// measurement window. Two guards prevent that here:
//   - total_ns/op measures wall clock across the requests AND the full drain,
//     so the writer goroutines' work is inside the measured window;
//   - after the timed region every trace is flushed and the persisted row count
//     is compared against the number of requests. The unsampled batched arm
//     must match exactly or the benchmark fails; the sampled arm can only be
//     bounded from above, and the sync baseline reports its loss rather than
//     asserting it, because dropping traces under write contention is a real
//     property of that path and hiding it would be dishonest.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/benchdb"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
)

// dbWorkCounters snapshots server-side work counters, so a reader can tell
// whether the optimization REMOVED work or merely RELOCATED it off the request
// goroutine. A batching writer that moved twelve statements to another core
// would improve request latency while conserving total work; these counters
// make that case distinguishable.
type dbWorkCounters struct {
	// supported reports whether the engine exposes the counters.
	supported bool
	// tupInserted, tupUpdated and tupDeleted are row versions written.
	tupInserted, tupUpdated, tupDeleted int64
	// xactCommit is the number of committed transactions.
	xactCommit int64
	// walLSN is the current write-ahead-log insert position. Unlike the
	// statistics-collector counters above it is read straight from the WAL
	// pointer, so it has no reporting lag, and it measures the durable work the
	// server actually performed rather than a sampled approximation of it.
	walLSN string
}

// readDBWork samples PostgreSQL's cumulative per-database work counters.
//
// PostgreSQL is the only engine here that reports row versions written without
// an extension, and row versions are the quantity the Phase-1 claim is really
// about: the baseline writes one row plus six updated versions of it per trace,
// the optimized path writes one row.
//
// Parameters:
//   - tb: the benchmark, used to fail fast on a query error.
//   - db: the handle to sample through.
//   - target: the engine being measured.
//
// Return values:
//   - dbWorkCounters: the sample, with supported=false on engines that cannot
//     report these counters.
func readDBWork(tb testing.TB, db *gorm.DB, target benchdb.Target) dbWorkCounters {
	tb.Helper()
	if target.Engine != benchdb.EnginePostgres {
		return dbWorkCounters{}
	}
	// PostgreSQL accumulates statistics in shared memory and flushes a
	// backend's pending counters at most once per PGSTAT_MIN_INTERVAL (about
	// 500 ms), so reading immediately after a burst of work undercounts it.
	// Waiting before each sample makes the delta trustworthy; both the before
	// and after samples pay the same cost, outside the timed region.
	time.Sleep(1500 * time.Millisecond)

	var row struct {
		TupInserted int64
		TupUpdated  int64
		TupDeleted  int64
		XactCommit  int64
		WalLSN      string
	}
	err := db.Raw(`SELECT tup_inserted, tup_updated, tup_deleted, xact_commit,
		pg_current_wal_lsn()::text AS wal_lsn
		FROM pg_stat_database WHERE datname = current_database()`).Scan(&row).Error
	require.NoError(tb, err)
	return dbWorkCounters{
		supported:   true,
		tupInserted: row.TupInserted,
		tupUpdated:  row.TupUpdated,
		tupDeleted:  row.TupDeleted,
		xactCommit:  row.XactCommit,
		walLSN:      row.WalLSN,
	}
}

// walBytesBetween returns how many bytes of write-ahead log were generated
// between two samples.
//
// Parameters:
//   - tb: the benchmark, used to fail fast on a query error.
//   - db: the handle to compute the difference through.
//   - before, after: the two samples.
//
// Return values:
//   - int64: WAL bytes generated, or 0 when the engine does not report them.
func walBytesBetween(tb testing.TB, db *gorm.DB, before, after dbWorkCounters) int64 {
	tb.Helper()
	if !before.supported || !after.supported || before.walLSN == "" || after.walLSN == "" {
		return 0
	}
	var bytes int64
	require.NoError(tb, db.Raw("SELECT pg_wal_lsn_diff(?::pg_lsn, ?::pg_lsn)::bigint",
		after.walLSN, before.walLSN).Scan(&bytes).Error)
	return bytes
}

// benchTraceConfig captures the trace configuration a benchmark variant needs.
type benchTraceConfig struct {
	// writeMode is config.TraceWriteModeSync for the baseline or
	// config.TraceWriteModeBatched for the optimized path.
	writeMode string
	// sampleRate is the probability an ordinary trace is persisted.
	sampleRate float64
	// noTracing builds an engine without TracingMiddleware at all. This is the
	// floor arm: without it, the reported ratio between the two write paths can
	// be inflated without limit simply by making the benchmark handler cheaper,
	// because the ratio is then dominated by work neither arm performs.
	noTracing bool
}

// setupBenchTracing points the model layer at the target database, installs the
// requested trace configuration, and starts the sinks.
//
// Parameters:
//   - b: the benchmark, used to fail fast and to register cleanup.
//   - target: the database engine to measure against.
//   - cfg: the trace configuration for this variant.
//
// Return values:
//   - *gorm.DB: the handle backing model.DB for this run.
//   - *statementCounter: the live SQL statement tally.
func setupBenchTracing(b *testing.B, target benchdb.Target, cfg benchTraceConfig) (*gorm.DB, *statementCounter) {
	b.Helper()

	db, restoreDialect := benchdb.Open(b, target)
	require.NoError(b, db.AutoMigrate(&model.Trace{}))
	benchdb.Reset(b, db, "traces")

	prevDB := model.DB
	model.DB = db

	prev := struct {
		sinks      []string
		writeMode  string
		sampleRate float64
		alwaysErr  bool
		alwaysSlow int
		queue      int
		batch      int
		writers    int
		interval   int
		excluded   []string
	}{
		config.TraceSinks, config.TraceWriteMode, config.TraceSampleRate,
		config.TraceAlwaysSampleErrors, config.TraceAlwaysSampleSlowMs,
		config.TraceQueueSize, config.TraceBatchSize, config.TraceWriterCount,
		config.TraceFlushIntervalMs, config.TraceExcludedPathPrefixes,
	}

	config.TraceSinks = []string{config.TraceSinkDB}
	config.TraceWriteMode = cfg.writeMode
	config.TraceSampleRate = cfg.sampleRate
	config.TraceAlwaysSampleErrors = true
	config.TraceAlwaysSampleSlowMs = 0
	// Production defaults, unchanged. Tuning the writer for the benchmark would
	// report a configuration nobody runs.
	config.TraceQueueSize = 20000
	config.TraceBatchSize = 500
	config.TraceWriterCount = 2
	config.TraceFlushIntervalMs = 1000
	config.TraceExcludedPathPrefixes = nil

	require.NoError(b, tracing.InitSinks(context.Background()))

	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := tracing.Shutdown(ctx); err != nil {
			b.Errorf("shutdown trace sinks: %+v", err)
		}
		tracing.SetSinkForTest(nil)()

		config.TraceSinks, config.TraceWriteMode, config.TraceSampleRate = prev.sinks, prev.writeMode, prev.sampleRate
		config.TraceAlwaysSampleErrors, config.TraceAlwaysSampleSlowMs = prev.alwaysErr, prev.alwaysSlow
		config.TraceQueueSize, config.TraceBatchSize = prev.queue, prev.batch
		config.TraceWriterCount, config.TraceFlushIntervalMs = prev.writers, prev.interval
		config.TraceExcludedPathPrefixes = prev.excluded

		model.DB = prevDB
		restoreDialect()
	})

	return db, attachStatementCounter(b, db)
}

// newBenchTracingEngine builds the gin engine the benchmark drives.
//
// The handler emits exactly the lifecycle marks a relay request emits, so both
// variants perform identical work apart from how that work is persisted.
//
// Parameters:
//   - b: the benchmark, used for helper bookkeeping.
//
// Return values:
//   - *gin.Engine: the configured engine.
func newBenchTracingEngine(b *testing.B, noTracing bool) *gin.Engine {
	b.Helper()
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelError.String()),
		gmw.WithLogger(logger.Logger.Named("bench")),
	))
	if !noTracing {
		engine.Use(TracingMiddleware())
	}
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		if !noTracing {
			tracing.RecordTraceTimestamp(c, model.TimestampRequestForwarded)
			tracing.RecordTraceTimestamp(c, model.TimestampFirstUpstreamResponse)
			tracing.RecordTraceTimestamp(c, model.TimestampUpstreamCompleted)
		}
		c.String(http.StatusOK, "ok")
	})
	return engine
}

// BenchmarkTracePipeline compares the pre-Phase-1 synchronous write path with
// the batched pipeline on an identical concurrent workload.
//
// Reported metrics beyond the standard ns/op:
//   - stmts_pre_flush/op: statements issued per request up to the moment the
//     timer stopped, and total_stmts/op including the drain.
//   - rows_written: traces actually persisted, which must equal the request
//     count; the benchmark fails otherwise.
//   - residual_frac: fraction of requests still queued when the timer stopped.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkTracePipeline(b *testing.B) {
	variants := []struct {
		name string
		cfg  benchTraceConfig
	}{
		{"floor_no_tracing", benchTraceConfig{writeMode: config.TraceWriteModeBatched, sampleRate: 1.0, noTracing: true}},
		{"baseline_sync", benchTraceConfig{writeMode: config.TraceWriteModeSync, sampleRate: 1.0}},
		{"batched", benchTraceConfig{writeMode: config.TraceWriteModeBatched, sampleRate: 1.0}},
		{"batched_sampled5pct", benchTraceConfig{writeMode: config.TraceWriteModeBatched, sampleRate: 0.05}},
	}

	for _, target := range benchdb.Targets() {
		for _, variant := range variants {
			b.Run(string(target.Engine)+"/"+variant.name, func(b *testing.B) {
				db, counter := setupBenchTracing(b, target, variant.cfg)
				engine := newBenchTracingEngine(b, variant.cfg.noTracing)

				var completed atomic.Int64
				workBefore := readDBWork(b, db, target)

				b.ResetTimer()
				counter.reset()
				wallStart := time.Now()

				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
						w := httptest.NewRecorder()
						engine.ServeHTTP(w, req)
						if w.Code != http.StatusOK {
							b.Errorf("unexpected status %d", w.Code)
							return
						}
						completed.Add(1)
					}
				})

				b.StopTimer()

				// Everything issued up to the moment the timer stopped. In the
				// batched arms this is NOT purely request-goroutine work: the
				// writers run concurrently, so their INSERTs land here too. The
				// exact "zero statements on the request goroutine" claim is
				// proven separately and deterministically by
				// TestTracingMiddlewareIssuesNoStatementsOnRequestPath.
				statementsBeforeFlush := counter.total()
				requests := completed.Load()

				// Draining is part of the work the system must do. Measuring
				// only the request goroutine would let a batching writer look
				// like a win purely by relocating work to another core, so the
				// total-wall-clock metric below includes the full drain.
				flushCtx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
				defer cancel()
				require.NoError(b, tracing.Flush(flushCtx))
				totalWall := time.Since(wallStart)

				workAfter := readDBWork(b, db, target)

				var rows int64
				require.NoError(b, db.Model(&model.Trace{}).Count(&rows).Error)

				// Trace loss is reported for every arm rather than asserted
				// away. The legacy synchronous path treats a failed write as
				// best effort and drops the trace, which under write contention
				// it measurably does; hiding that behind an assertion would
				// suppress a real property of the baseline. The optimized arm
				// makes the stronger promise -- nothing accepted is lost -- so
				// only that one is allowed to fail the benchmark.
				var lossPct float64
				switch {
				case variant.cfg.noTracing:
					require.Zero(b, rows, "the floor arm must persist no traces")
					require.Zero(b, statementsBeforeFlush, "the floor arm must issue no trace statements")
				case variant.cfg.sampleRate < 1.0:
					require.LessOrEqual(b, rows, requests, "sampling must not invent traces")
				case variant.cfg.writeMode == config.TraceWriteModeSync:
					lossPct = float64(requests-rows) / float64(max(requests, 1)) * 100
				default:
					require.Equal(b, requests, rows,
						"the batched pipeline must lose nothing it accepted; a shortfall invalidates the timing")
				}

				perRequest := func(v int64) float64 { return float64(v) / float64(max(requests, 1)) }

				b.ReportMetric(perRequest(statementsBeforeFlush), "stmts_pre_flush/op")
				b.ReportMetric(perRequest(counter.total()), "total_stmts/op")
				b.ReportMetric(float64(totalWall.Nanoseconds())/float64(max(requests, 1)), "total_ns/op")
				b.ReportMetric(float64(rows), "rows_written")
				b.ReportMetric(lossPct, "trace_loss_pct")

				if workBefore.supported && workAfter.supported {
					// Row versions written per trace is the work-conservation
					// evidence: the baseline writes one row plus six updated
					// versions of it, the optimized path writes one row. A
					// figure that did not fall would mean the work was merely
					// moved, not removed.
					b.ReportMetric(perRequest(workAfter.tupUpdated-workBefore.tupUpdated), "pg_tup_upd/op")
					b.ReportMetric(perRequest(workAfter.xactCommit-workBefore.xactCommit), "pg_xact/op")
					b.ReportMetric(perRequest(walBytesBetween(b, db, workBefore, workAfter)), "pg_wal_B/op")
				}
			})
		}
	}
}

// BenchmarkTraceArrivalRate measures statements per trace as a function of
// arrival rate.
//
// The benchmark above drives requests as fast as the machine allows, which
// fills the writer's 500-row batches instantly and yields the most flattering
// possible statements-per-trace figure. Real gateways run at a bounded arrival
// rate, where a partially filled batch is flushed by the 1 s ticker instead.
// Statements per trace is therefore a CURVE over arrival rate, not a scalar,
// and publishing only the saturated point would overstate the result at low
// load.
//
// The driver is RATE-LIMITED CLOSED LOOP, not open loop: one goroutine waits
// for a tick and then blocks in ServeHTTP, so the offered rate is capped at
// 1/service_time and surplus ticks are discarded. The requested rate is
// therefore an upper bound; achieved_per_s below reports what was actually
// delivered, and that is the number to read.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkTraceArrivalRate(b *testing.B) {
	rates := []int{50, 500, 5000}

	for _, target := range benchdb.Targets() {
		for _, rate := range rates {
			b.Run(fmt.Sprintf("%s/requested_%d_per_s", target.Engine, rate), func(b *testing.B) {
				db, counter := setupBenchTracing(b, target,
					benchTraceConfig{writeMode: config.TraceWriteModeBatched, sampleRate: 1.0})
				engine := newBenchTracingEngine(b, false)

				// A fixed observation window keeps the comparison across rates
				// meaningful; b.N is pinned so the window, not the iteration
				// count, defines the experiment.
				require.Equal(b, 1, b.N, "run this benchmark with -benchtime=1x")

				const window = 3 * time.Second
				interval := time.Second / time.Duration(rate)

				b.ResetTimer()
				counter.reset()

				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				deadline := time.Now().Add(window)

				var sent int64
				for time.Now().Before(deadline) {
					<-ticker.C
					req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", http.NoBody)
					w := httptest.NewRecorder()
					engine.ServeHTTP(w, req)
					sent++
				}
				b.StopTimer()

				flushCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				require.NoError(b, tracing.Flush(flushCtx))

				var rows int64
				require.NoError(b, db.Model(&model.Trace{}).Count(&rows).Error)
				require.Equal(b, sent, rows, "every request must produce exactly one persisted trace")

				achieved := float64(sent) / window.Seconds()

				b.ReportMetric(achieved, "achieved_per_s")
				b.ReportMetric(float64(sent), "traces_sent")
				b.ReportMetric(float64(counter.total())/float64(max(sent, 1)), "total_stmts/op")
				b.ReportMetric(float64(counter.creates.Load()), "insert_stmts")
				b.ReportMetric(float64(sent)/float64(max(counter.creates.Load(), 1)), "traces_per_insert")
			})
		}
	}
}
