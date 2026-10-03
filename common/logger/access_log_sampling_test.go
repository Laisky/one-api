package logger

// Sampling through the REAL gin access logger (W0.5).
//
// sampling_test.go drives the sampler directly with fixed message strings,
// which proves the core works but proves nothing about production: zap's
// sampler keys on (level, message), so whether sampling does anything at all in
// this gateway depends entirely on whether the access line puts the URL and the
// request id in the zap MESSAGE or in FIELDS. If they are in the message, every
// request produces a unique key, every key gets its own initial budget, and
// sampling degenerates into an expensive no-op.
//
// These tests establish the answer against the real middleware
// (gmw.NewLoggerMiddleware, wired as main.go wires it) with changing URLs and
// changing trace ids, and then assert the behavior that follows from it.
//
// The answer: the message is buildStatus(), i.e. "<status> <method>" -- stable
// across URLs and request ids -- and url/remote/host/trace_id/cost are attached
// with logger.With(...) as fields. Sampling is therefore real, and it is keyed
// by status-and-method, so a 200 flood is thinned while a 500 keeps its own
// budget. Nothing in the message needs fixing.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// newSampledAccessLogger builds the process logger as SetupEnhancedLogger does
// -- the real sampler installed over the real core -- and returns what reaches
// the writer.
//
// Parameters:
//   - t: the test handle; sampling configuration is restored on cleanup.
//   - first: entries kept per (level, message) per tick.
//   - thereafter: thinning factor after the initial budget.
//
// Return values:
//   - glog.Logger: the logger to hand to the gin middleware.
//   - *recordingCore: every entry that survived sampling.
func newSampledAccessLogger(t *testing.T, first, thereafter int) (glog.Logger, *recordingCore) {
	t.Helper()

	prevInitial, prevThereafter, prevTick := config.LogSampleInitial, config.LogSampleThereafter, config.LogSampleTickMs
	config.LogSampleInitial = first
	config.LogSampleThereafter = thereafter
	// One hour: the tick must not roll during the test, or the budget refills
	// and the assertions become timing-dependent.
	config.LogSampleTickMs = 3600000
	t.Cleanup(func() {
		config.LogSampleInitial, config.LogSampleThereafter, config.LogSampleTickMs = prevInitial, prevThereafter, prevTick
	})

	opt, ok := samplingOption()
	require.True(t, ok, "sampling must be configured for this test to mean anything")

	base := newRecordingCore()
	return loggerOverCore(t, base).WithOptions(opt), base
}

// newAccessLogEngine wires the real access logger in front of a trivial route,
// the way main.go wires it.
//
// Parameters:
//   - t: the test handle.
//   - lg: the logger the middleware writes through.
//   - colored: whether to enable the colored status line main.go uses.
//   - handler: extra work to run inside the request, or nil.
//
// Return values:
//   - *gin.Engine: the engine to drive with requests.
func newAccessLogEngine(t *testing.T, lg glog.Logger, colored bool, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	opts := []gmw.LoggerMwOptFunc{
		gmw.WithLevel(glog.LevelInfo.String()),
		gmw.WithLogger(lg),
	}
	if colored {
		opts = append(opts, gmw.WithLoggerMwColored())
	}

	engine := gin.New()
	engine.Use(gmw.NewLoggerMiddleware(opts...))
	engine.GET("/v1/models/:model", func(c *gin.Context) {
		if handler != nil {
			handler(c)
		}
		c.String(http.StatusOK, "ok")
	})
	engine.GET("/v1/missing", func(c *gin.Context) {
		c.String(http.StatusNotFound, "nope")
	})
	return engine
}

// driveRequests issues n requests with a DIFFERENT URL and a DIFFERENT trace id
// each time, which is the whole point: constant traffic against one endpoint
// still varies in exactly the two values that would poison a message-keyed
// sampler.
//
// Parameters:
//   - t: the test handle.
//   - engine: the engine to drive.
//   - path: a printf template taking the iteration index.
//   - n: how many requests to issue.
//
// Return values: none.
func driveRequests(t *testing.T, engine *gin.Engine, path string, n int) {
	t.Helper()
	for i := range n {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf(path, i), nil)
		req.Header.Set(gutils.TracingKey.String(), fmt.Sprintf("trace-%d:%d:0:1", i, i))
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
	}
}

// accessLines returns the access status lines, ignoring anything the handler
// logged.
//
// The discriminator is the response_size field, which the middleware attaches
// only AFTER ctx.Next() returns. url and trace_id cannot be used: the
// middleware binds them to the per-request logger, so every line a handler
// emits carries them too.
//
// Parameters:
//   - entries: everything the core recorded.
//
// Return values:
//   - []recordedEntry: the access lines.
func accessLines(entries []recordedEntry) []recordedEntry {
	out := make([]recordedEntry, 0, len(entries))
	for _, e := range entries {
		if _, ok := fieldValue(e, "response_size"); ok {
			out = append(out, e)
		}
	}
	return out
}

// fieldValue returns a field's string value.
//
// Parameters:
//   - entry: the recorded entry.
//   - key: the field name.
//
// Return values:
//   - string: the field's string value.
//   - bool: whether the field is present.
func fieldValue(entry recordedEntry, key string) (string, bool) {
	for _, f := range entry.Fields {
		if f.Key == key {
			return f.String, true
		}
	}
	return "", false
}

// TestAccessLogKeepsVariableValuesInFields establishes the fact everything else
// in this file depends on: the access line's MESSAGE is stable, and the URL and
// trace id -- the values that change on every request -- are fields.
//
// If this ever regresses, sampling silently becomes a no-op: every request
// would present the sampler with a brand-new key and receive a fresh budget.
func TestAccessLogKeepsVariableValuesInFields(t *testing.T) {
	lg, base := newSampledAccessLogger(t, 1000, 1)
	engine := newAccessLogEngine(t, lg, false, nil)

	driveRequests(t, engine, "/v1/models/gpt-%d?attempt=%[1]d", 5)

	lines := accessLines(base.snapshot())
	require.Len(t, lines, 5)

	urls := map[string]struct{}{}
	traces := map[string]struct{}{}
	for _, line := range lines {
		require.Equal(t, "200 GET", line.Message,
			"the access line message must stay stable, or the (level, message) sampler cannot thin it")

		url, ok := fieldValue(line, "url")
		require.True(t, ok, "the URL must be a field, not part of the message")
		urls[url] = struct{}{}

		trace, ok := fieldValue(line, "trace_id")
		require.True(t, ok, "the request id must be a field, not part of the message")
		traces[trace] = struct{}{}
	}

	require.Len(t, urls, 5, "every request used a different URL")
	require.Len(t, traces, 5, "every request used a different trace id")
}

// TestAccessLogSamplingThinsRealRequests is the behavior that follows: a flood
// of successful requests, all with distinct URLs and trace ids, is thinned to
// the configured budget.
//
// It also proves the per-request logger derived by the middleware with
// logger.With(...) keeps SHARING the sampler's counters. If With reset them,
// every request would carry a private budget and all 60 lines would survive.
func TestAccessLogSamplingThinsRealRequests(t *testing.T) {
	lg, base := newSampledAccessLogger(t, 3, 0)
	engine := newAccessLogEngine(t, lg, false, nil)

	driveRequests(t, engine, "/v1/models/gpt-%d", 60)

	require.Len(t, accessLines(base.snapshot()), 3,
		"changing URLs and request ids must not buy a fresh sampling budget")
}

// TestAccessLogSamplingThereafterKeepsEveryNth verifies the thinning factor
// through the real middleware rather than through a synthetic message.
func TestAccessLogSamplingThereafterKeepsEveryNth(t *testing.T) {
	lg, base := newSampledAccessLogger(t, 2, 10)
	engine := newAccessLogEngine(t, lg, false, nil)

	driveRequests(t, engine, "/v1/models/gpt-%d", 42)

	// 2 from the initial budget, then one in every 10 of the remaining 40.
	require.Len(t, accessLines(base.snapshot()), 6)
}

// TestAccessLogSamplingSurvivesColoredStatus verifies the production wiring:
// main.go enables gmw.WithLoggerMwColored, which wraps the status line in ANSI
// escapes. Those escapes are constant per status class, so the message stays
// stable and sampling still applies.
func TestAccessLogSamplingSurvivesColoredStatus(t *testing.T) {
	lg, base := newSampledAccessLogger(t, 3, 0)
	engine := newAccessLogEngine(t, lg, true, nil)

	driveRequests(t, engine, "/v1/models/gpt-%d", 60)

	lines := accessLines(base.snapshot())
	require.Len(t, lines, 3, "colored status lines must sample exactly like plain ones")
	require.Contains(t, lines[0].Message, "200 GET")
	require.NotEqual(t, "200 GET", lines[0].Message, "the colored line carries ANSI escapes")
}

// TestAccessLogSamplingSeparatesStatusCodes verifies the sampler key is the
// status line, so a rare failure is not starved by a flood of successes: each
// distinct "<status> <method>" keeps its own budget.
func TestAccessLogSamplingSeparatesStatusCodes(t *testing.T) {
	lg, base := newSampledAccessLogger(t, 2, 0)
	engine := newAccessLogEngine(t, lg, false, nil)

	driveRequests(t, engine, "/v1/models/gpt-%d", 40)
	driveRequests(t, engine, "/v1/missing?attempt=%d", 40)

	counts := map[string]int{}
	for _, line := range accessLines(base.snapshot()) {
		counts[line.Message]++
	}

	require.Equal(t, 2, counts["200 GET"])
	require.Equal(t, 2, counts["404 GET"], "a distinct status keeps its own budget")
}

// TestAccessLogNeverThinsWarnAndAbove is the guarantee that must hold through
// the real request path too: a repeated warning or error reports the SCALE of
// an incident, and thinning it would misreport how bad things are.
func TestAccessLogNeverThinsWarnAndAbove(t *testing.T) {
	lg, base := newSampledAccessLogger(t, 1, 0)
	engine := newAccessLogEngine(t, lg, false, func(c *gin.Context) {
		// Emitted through the per-request logger the middleware installed, so
		// it carries the request's fields and the same sampler.
		requestLogger := gmw.GetLogger(c)
		requestLogger.Warn("upstream channel is failing", zap.String("channel", "42"))
		requestLogger.Error("upstream request failed", zap.String("channel", "42"))
	})

	driveRequests(t, engine, "/v1/models/gpt-%d", 30)

	warns, errs := 0, 0
	for _, e := range base.snapshot() {
		switch e.Message {
		case "upstream channel is failing":
			warns++
		case "upstream request failed":
			errs++
		}
	}

	require.Equal(t, 30, warns, "warn must pass through unsampled on the real request path")
	require.Equal(t, 30, errs, "error must pass through unsampled on the real request path")
	require.Equal(t, 1, len(accessLines(base.snapshot())), "the INFO access line is still thinned")
}
