package model

// Retention sweep benchmark (proposal
// docs/proposals/20260905_observability-data-tiering.md, Phase 0 / W0.2).
//
// WHAT IS AND IS NOT CLAIMED
//
// Chunked deletion is NOT claimed to be faster. It is expected to be SLOWER in
// total wall clock: it issues more statements and, at the production default,
// deliberately sleeps RETENTION_DELETE_PAUSE_MS between them. The claim is
// about boundedness -- that no single statement holds locks or accumulates redo
// for the whole sweep -- and the operator-visible consequence: the gateway
// keeps serving while retention runs.
//
// This benchmark therefore reports, for each variant:
//   - sweep_ms:       total sweep duration (chunked is expected to be WORSE)
//   - max_stmt_ms:    longest single DELETE statement (the boundedness claim)
//   - fg_p99_ms:      p99 latency of a concurrent foreground workload, and
//   - fg_errors:      foreground failures (lock timeouts, SQLITE_BUSY)
//     observed while the sweep runs (the claim that actually matters)
//
// The pause is separated from the chunking by a third variant with
// RETENTION_DELETE_PAUSE_MS=0, so a reader can tell whether any improvement
// comes from bounding the statement or merely from sleeping.

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/benchdb"
	"github.com/Laisky/one-api/common/config"
)

// retentionSeedRows is how many expired rows a sweep must remove. It is large
// enough that an unbounded DELETE is clearly a long-running transaction and
// small enough that seeding stays practical.
const retentionSeedRows = 200_000

// retentionFreshRows is how many non-expired rows must survive the sweep, so a
// variant that deletes too much is caught rather than rewarded.
const retentionFreshRows = 5_000

// unboundedDeleteExpiredTraces is the pre-proposal retention statement,
// reproduced verbatim as the benchmark baseline.
//
// TestRetentionBaselineFidelity pins this against the historical source so it
// cannot silently drift into a strawman.
//
// Parameters:
//   - ctx: cancellation scope for the statement.
//   - db: the handle owning the traces table.
//   - cutoff: exclusive upper bound on created_at, in Unix milliseconds.
//
// Return values:
//   - int64: rows removed.
//   - error: the statement's error, if any.
func unboundedDeleteExpiredTraces(ctx context.Context, db *gorm.DB, cutoff int64) (int64, error) {
	tx := db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&Trace{})
	return tx.RowsAffected, tx.Error
}

// TestRetentionBaselineFidelity pins the benchmark baseline to the real
// pre-proposal source.
//
// The benchmark's credibility rests entirely on its baseline being the code
// that actually shipped. This test reads that code out of git history and fails
// if the reproduction here has drifted from it.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestRetentionBaselineFidelity(t *testing.T) {
	out, err := exec.Command("git", "show", "397781e1:model/trace_retention.go").Output()
	if err != nil {
		t.Skipf("baseline commit not reachable from this checkout: %v", err)
	}
	historical := string(out)

	require.Contains(t, historical, `DB.Where("created_at < ?", cutoff).Delete(&Trace{})`,
		"the historical retention statement changed; update unboundedDeleteExpiredTraces before trusting the benchmark")
	require.NotContains(t, historical, "LIMIT",
		"the historical sweep must be unbounded for this baseline to be meaningful")
}

// seedRetentionTraceRows inserts expired and fresh trace rows.
//
// Parameters:
//   - tb: the test or benchmark, used to fail fast.
//   - db: the handle owning the traces table.
//   - expired: how many rows must fall inside the retention predicate.
//   - fresh: how many rows must fall outside it.
//   - cutoff: the retention boundary, in Unix milliseconds.
//
// Return values: none.
func seedRetentionTraceRows(tb testing.TB, db *gorm.DB, expired, fresh int, cutoff int64) {
	tb.Helper()

	const perStatement = 1000
	insert := func(count int, baseTS int64, idPrefix string) {
		for start := 0; start < count; start += perStatement {
			end := min(start+perStatement, count)
			var sb strings.Builder
			sb.WriteString("INSERT INTO traces (uuid, trace_id, url, method, body_size, status, timestamps, created_at, updated_at) VALUES ")
			args := make([]any, 0, (end-start)*9)
			for i := start; i < end; i++ {
				if i > start {
					sb.WriteString(",")
				}
				sb.WriteString("(?,?,?,?,?,?,?,?,?)")
				args = append(args,
					fmt.Sprintf("%s-uuid-%09d", idPrefix, i),
					fmt.Sprintf("%s-%09d", idPrefix, i),
					"/v1/chat/completions",
					"POST",
					int64(1024),
					200,
					`{"request_received":1,"request_forwarded":2,"first_upstream_response":3,"first_client_response":4,"upstream_completed":5,"request_completed":6}`,
					baseTS+int64(i%1000),
					baseTS,
				)
			}
			require.NoError(tb, db.Exec(sb.String(), args...).Error)
		}
	}

	insert(expired, cutoff-10_000_000, "expired")
	insert(fresh, cutoff+10_000_000, "fresh")
}

// statementTimer records how long each DELETE statement took, so the longest
// one can be reported.
type statementTimer struct {
	mu        sync.Mutex
	durations []time.Duration
}

// record appends one statement duration.
//
// Parameters:
//   - d: the observed duration.
//
// Return values: none.
func (s *statementTimer) record(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.durations = append(s.durations, d)
}

// percentiles returns the median, p99 and maximum recorded statement duration.
//
// The maximum alone is a poor summary: one cold first statement dominates it.
// Reporting the median alongside shows whether a long statement is the norm or
// an outlier, which is the difference between "this sweep holds locks" and
// "this sweep touched a cold page once".
//
// Parameters: none.
//
// Return values:
//   - time.Duration: the median statement duration.
//   - time.Duration: the p99 statement duration.
//   - time.Duration: the longest statement duration.
func (s *statementTimer) percentiles() (median, p99, worst time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.durations) == 0 {
		return 0, 0, 0
	}
	sorted := make([]time.Duration, len(s.durations))
	copy(sorted, s.durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	n := len(sorted)
	return sorted[n*50/100], sorted[min(n*99/100, n-1)], sorted[n-1]
}

// count returns how many statements were recorded.
//
// Parameters: none.
//
// Return values:
//   - int: the statement count.
func (s *statementTimer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.durations)
}

// foregroundResult summarizes the concurrent workload observed during a sweep.
type foregroundResult struct {
	// ops is how many foreground operations completed.
	ops int64
	// errs is how many foreground operations failed.
	errs int64
	// p50, p99 and worst are latency percentiles across completed operations.
	p50, p99, worst time.Duration
	// slowShare is the fraction of operations slower than foregroundSlowMs.
	//
	// Percentiles and maxima are drawn from very different sample counts across
	// variants (a longer sweep observes more foreground operations), so a share
	// is the comparable summary: it answers "what proportion of requests felt
	// bad", independent of how many there were.
	slowShare float64
}

// foregroundSlowMs is the latency above which a simulated gateway operation is
// counted as degraded.
const foregroundSlowMs = 25

// foregroundOfferedRate is the request rate the simulated gateway offers during
// a sweep, in operations per second.
//
// It is FIXED rather than closed loop. A closed-loop generator issues as many
// operations as the database will accept, so a sweep that runs four times longer
// observes roughly four times more operations: both the numerator and the
// denominator of any degradation measure then depend on the variant, and no two
// arms are comparable. With a fixed offered rate, the degraded-operation count
// measures total harm and the degraded fraction measures per-request risk, and
// both mean the same thing in every arm.
const foregroundOfferedRate = 400

// runForegroundLoad offers a fixed rate of live-gateway-like traffic against the
// same table the sweep is deleting from, until stop is closed.
//
// This is the measurement that matters: an operator does not care that a sweep
// takes longer, they care whether requests keep completing while it runs.
//
// Parameters:
//   - db: the handle to issue foreground work against.
//   - workers: how many concurrent clients to simulate.
//   - stop: closed to end the load.
//
// Return values:
//   - <-chan foregroundResult: receives the summary once the load has stopped.
func runForegroundLoad(db *gorm.DB, workers int, stop <-chan struct{}) <-chan foregroundResult {
	done := make(chan foregroundResult, 1)

	var (
		mu       sync.Mutex
		latency  []time.Duration
		ops      atomic.Int64
		errs     atomic.Int64
		wg       sync.WaitGroup
		sequence atomic.Int64
	)

	perWorkerInterval := time.Duration(workers) * time.Second / foregroundOfferedRate

	for w := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()

			ticker := time.NewTicker(perWorkerInterval)
			defer ticker.Stop()

			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
				}

				n := sequence.Add(1)
				start := time.Now()
				err := db.Exec(
					"INSERT INTO traces (uuid, trace_id, url, method, body_size, status, timestamps, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
					fmt.Sprintf("fg-uuid-%d-%d", worker, n),
					fmt.Sprintf("fg-%d-%d", worker, n),
					"/v1/chat/completions", "POST", int64(512), 200, "{}",
					time.Now().UTC().UnixMilli(), time.Now().UTC().UnixMilli(),
				).Error
				elapsed := time.Since(start)

				if err != nil {
					errs.Add(1)
					continue
				}
				ops.Add(1)
				mu.Lock()
				latency = append(latency, elapsed)
				mu.Unlock()
			}
		}(w)
	}

	go func() {
		wg.Wait()
		mu.Lock()
		defer mu.Unlock()
		sort.Slice(latency, func(i, j int) bool { return latency[i] < latency[j] })

		res := foregroundResult{ops: ops.Load(), errs: errs.Load()}
		if n := len(latency); n > 0 {
			res.p50 = latency[n*50/100]
			res.p99 = latency[min(n*99/100, n-1)]
			res.worst = latency[n-1]

			slow := 0
			for _, d := range latency {
				if d >= foregroundSlowMs*time.Millisecond {
					slow++
				}
			}
			res.slowShare = float64(slow) / float64(n)
		}
		done <- res
	}()

	return done
}

// retentionPauseSweep enumerates the inter-chunk pauses measured, in
// milliseconds, so the production default can be read off a curve.
var retentionPauseSweep = []int{0, 10, 25, 50, 100, 200}

// retentionVariant describes one sweep implementation under measurement.
type retentionVariant struct {
	// name identifies the variant in benchmark output.
	name string
	// pauseMs is the inter-chunk pause; ignored by the unbounded variant.
	pauseMs int
	// run executes the sweep and returns how many rows it removed.
	run func(ctx context.Context, db *gorm.DB, cutoff int64) (int64, error)
}

// BenchmarkRetentionSweep compares the pre-proposal unbounded retention DELETE
// with the chunked sweeper, under a concurrent foreground workload.
//
// Parameters:
//   - b: the benchmark handle.
//
// Return values: none.
func BenchmarkRetentionSweep(b *testing.B) {
	variants := []retentionVariant{{
		name: "baseline_unbounded",
		run: func(ctx context.Context, db *gorm.DB, cutoff int64) (int64, error) {
			return unboundedDeleteExpiredTraces(ctx, db, cutoff)
		},
	}}

	// Sweep the inter-chunk pause rather than testing two arbitrary points: the
	// production default has to be chosen from a curve, and the pause trades
	// sweep duration (how long the disruption window lasts) against yielding
	// (how much of that window the sweeper is idle).
	for _, pauseMs := range retentionPauseSweep {
		variants = append(variants, retentionVariant{
			name:    fmt.Sprintf("chunked_pause%dms", pauseMs),
			pauseMs: pauseMs,
			run: func(ctx context.Context, db *gorm.DB, cutoff int64) (int64, error) {
				return ChunkedDelete(ctx, db, ChunkedDeleteOptions{
					Table: "traces", Where: "created_at < ?", Args: []any{cutoff},
					BatchSize: config.RetentionDeleteBatchSize,
					Pause:     time.Duration(pauseMs) * time.Millisecond,
				})
			},
		})
	}

	for _, target := range benchdb.Targets() {
		for _, variant := range variants {
			b.Run(string(target.Engine)+"/"+variant.name, func(b *testing.B) {
				// Two handles to the same database: the sweep runs on the
				// instrumented one, the foreground load on an uninstrumented
				// one. Sharing a handle would time every foreground INSERT as
				// though it were part of the sweep and destroy the statement
				// percentiles.
				db, fgDB, restore := benchdb.OpenPair(b, target)
				defer restore()
				require.NoError(b, db.AutoMigrate(&Trace{}))

				timer := &statementTimer{}
				require.NoError(b, db.Callback().Raw().Before("gorm:raw").
					Register("bench:stmt_start", func(tx *gorm.DB) { tx.Set("bench:start", time.Now()) }))
				require.NoError(b, db.Callback().Raw().After("gorm:raw").
					Register("bench:stmt_end", func(tx *gorm.DB) {
						if v, ok := tx.Get("bench:start"); ok {
							timer.record(time.Since(v.(time.Time)))
						}
					}))
				require.NoError(b, db.Callback().Delete().Before("gorm:delete").
					Register("bench:del_start", func(tx *gorm.DB) { tx.Set("bench:start", time.Now()) }))
				require.NoError(b, db.Callback().Delete().After("gorm:delete").
					Register("bench:del_end", func(tx *gorm.DB) {
						if v, ok := tx.Get("bench:start"); ok {
							timer.record(time.Since(v.(time.Time)))
						}
					}))

				cutoff := time.Now().UTC().UnixMilli()

				var (
					sweepTotal                  time.Duration
					stmtMedian, stmtWorst       time.Duration
					stmtCount                   int
					totalDeleted                int64
					totalFgOps, totalFgDegraded int64
					lastFg                      foregroundResult
				)

				for range b.N {
					b.StopTimer()
					benchdb.Reset(b, db, "traces")
					seedRetentionTraceRows(b, db, retentionSeedRows, retentionFreshRows, cutoff)
					timer.mu.Lock()
					timer.durations = nil
					timer.mu.Unlock()

					stop := make(chan struct{})
					fgDone := runForegroundLoad(fgDB, 4, stop)

					b.StartTimer()
					start := time.Now()
					n, err := variant.run(context.Background(), db, cutoff)
					elapsed := time.Since(start)
					b.StopTimer()

					close(stop)
					fg := <-fgDone

					require.NoError(b, err)
					totalDeleted += n
					sweepTotal += elapsed
					totalFgOps += fg.ops
					totalFgDegraded += int64(fg.slowShare*float64(fg.ops) + 0.5)
					lastFg = fg
					stmtMedian, _, stmtWorst = timer.percentiles()
					stmtCount = timer.count()

					var survivors int64
					require.NoError(b, db.Model(&Trace{}).Where("created_at >= ?", cutoff).Count(&survivors).Error)
					require.GreaterOrEqual(b, survivors, int64(retentionFreshRows),
						"a sweep must never remove rows outside its predicate")
					b.StartTimer()
				}

				// Observed delete throughput. This is total rows removed over
				// total sweep wall time on a fully cached table under competing
				// load -- an UPPER BOUND on steady-state capacity, not the
				// steady-state rate itself: production tables are far larger
				// than the buffer pool and a sweep removes a much smaller
				// fraction of them.
				throughput := float64(totalDeleted) / sweepTotal.Seconds()

				b.ReportMetric(float64(sweepTotal.Milliseconds())/float64(b.N), "sweep_ms/op")
				b.ReportMetric(throughput, "delete_rows_per_s")
				b.ReportMetric(float64(stmtMedian.Microseconds())/1000, "stmt_p50_ms")
				b.ReportMetric(float64(stmtWorst.Microseconds())/1000, "stmt_max_ms")
				b.ReportMetric(float64(stmtCount), "stmts")
				b.ReportMetric(float64(totalDeleted)/float64(b.N), "rows_deleted")

				// The foreground generator is CLOSED LOOP, so both the operation
				// count and the share of slow operations are endogenous: a
				// longer sweep observes more operations. Neither a percentile
				// nor a share is comparable across variants. The absolute count
				// of degraded operations per sweep is, because every arm removes
				// the same number of rows.
				b.ReportMetric(float64(totalFgDegraded)/float64(b.N), "fg_degraded_ops")
				if totalFgOps > 0 {
					b.ReportMetric(float64(totalFgDegraded)/float64(totalFgOps)*100, "fg_degraded_pct")
				}
				b.ReportMetric(float64(totalFgOps)/float64(b.N), "fg_ops")
				b.ReportMetric(float64(lastFg.p50.Microseconds())/1000, "fg_p50_ms")
				b.ReportMetric(float64(lastFg.errs), "fg_errors")
			})
		}
	}
}
