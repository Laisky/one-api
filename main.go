package main

import (
	"context"
	"embed"
	"encoding/base64"
	"net"
	"net/http"
	nhpprof "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	_ "github.com/joho/godotenv/autoload"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/telemetry"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/controller"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/monitor"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/mcp"
	responsestate "github.com/Laisky/one-api/relay/state"
	"github.com/Laisky/one-api/router"
)

//go:embed web/build/*

var buildFS embed.FS

func main() {
	ctx := context.Background()

	common.Init()
	logger.SetupLogger()

	// Setup enhanced logger with alertPusher integration
	logger.SetupEnhancedLogger(ctx)

	// workerCtx bounds every periodic background worker started below. They are
	// producers of database and log-directory work, so they must be stoppable:
	// with a never-cancelled context their tickers kept firing throughout
	// shutdown and after model.CloseDB, issuing statements against a closed
	// pool. The shutdown sequence cancels this context (the retention_workers
	// step) before the trace sinks close and long before the database does, and
	// then JOINS the retention cleaners so unfinished sweeps are reported rather
	// than raced. Cancellation is not deferred here on purpose: every
	// logger.Logger.Fatal below calls os.Exit and no defer would run anyway.
	workerCtx, stopBackgroundWorkers := context.WithCancel(ctx)
	controller.SetDashboardAggregateLifecycleContext(workerCtx)

	// Started after the enhanced logger is installed: the worker captures the
	// logger it will use for the life of the process, and starting it earlier
	// would both race SetupEnhancedLogger's write to the global logger and
	// leave the worker without the alert hook.
	logger.StartLogRetentionCleaner(workerCtx, config.LogRetentionDays, logger.LogDir)

	var (
		err           error
		otelProviders *telemetry.ProviderBundle
	)

	logger.Logger.Info("One API started", zap.String("version", common.Version))

	if config.GinMode != gin.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}

	// OpenTelemetry is initialized BEFORE tracing.InitSinks below, and the order
	// is load-bearing: an OTLP trace sink built while the process still carries
	// OpenTelemetry's global no-op provider exports nothing, and would report
	// export_failed once per request forever. InitSinks now refuses to build the
	// sink unless telemetry.ProviderInitialized() is true, so reversing these
	// two blocks fails startup instead of degrading silently.
	if config.OpenTelemetryEnabled {
		otelProviders, err = telemetry.InitOpenTelemetry(ctx)
		if err != nil {
			logger.Logger.Fatal("failed to initialize OpenTelemetry", zap.Error(err))
		}
	}

	// check theme
	logger.Logger.Info("using theme", zap.String("theme", config.Theme))
	if err := isThemeValid(); err != nil {
		logger.Logger.Fatal("invalid theme", zap.Error(err))
	}

	// Initialize SQL Database. The bootstrap orchestrator initializes both schemas,
	// constructs the explicit database topology, and runs external UUID reconciliation
	// exactly once, so the InitDB/InitLogDB compatibility wrappers are not used here.
	if err := model.InitDatabases(ctx); err != nil {
		logger.Logger.Fatal("database bootstrap error", zap.Error(err))
	}
	model.StartTraceRetentionCleaner(workerCtx, config.TraceRetentionDays)

	// Trace sinks own the asynchronous batched writer, so they must start after
	// the database handles exist and before the HTTP server accepts requests.
	if err := tracing.InitSinks(ctx); err != nil {
		logger.Logger.Fatal("failed to initialize trace sinks", zap.Error(err))
	}
	model.StartAsyncTaskRetentionCleaner(workerCtx, config.AsyncTaskRetentionDays)
	err = model.CreateRootAccountIfNeed()
	if err != nil {
		logger.Logger.Fatal("database init error", zap.Error(err))
	}
	// The database is deliberately not closed by a defer here. Every
	// logger.Logger.Fatal below calls os.Exit, so deferred functions never run on
	// the failure paths such a defer would look like it protects; it would only
	// ever fire on a normal shutdown, right after the ordered close at the end of
	// main, closing the same handles a second time. The single close is the last
	// step of the shutdown sequence built by newShutdownSequence.

	// Initialize Redis
	err = common.InitRedisClient()
	if err != nil {
		logger.Logger.Fatal("failed to initialize Redis", zap.Error(err))
	}

	// Initialize the gateway Responses state layer. When RESPONSE_STATE_ENABLED is
	// false this is a no-op and current behavior is preserved. When enabled it
	// refuses to start without a healthy Redis backend and a stable encryption key
	// rather than degrading to an in-process store.
	if err = responsestate.Init(); err != nil {
		logger.Logger.Fatal("failed to initialize response state layer", zap.Error(err))
	}

	// Initialize options
	model.InitOptionMap()
	if common.IsRedisEnabled() {
		// for compatibility with old versions
		config.MemoryCacheEnabled = true
	}
	if config.MemoryCacheEnabled {
		logger.Logger.Info("memory cache enabled", zap.Int("sync_frequency", config.SyncFrequency))
		model.InitChannelCache()
	}
	if config.MemoryCacheEnabled {
		model.StartBackgroundWorker(workerCtx, func(ctx context.Context) {
			model.SyncOptionsContext(ctx, config.SyncFrequency)
		})
		model.StartBackgroundWorker(workerCtx, func(ctx context.Context) {
			model.SyncChannelCacheContext(ctx, config.SyncFrequency)
		})
	}
	mcp.StartAutoSync(workerCtx)
	if config.ChannelTestFrequency > 0 {
		model.StartBackgroundWorker(workerCtx, func(ctx context.Context) {
			controller.AutomaticallyTestChannelsContext(ctx, config.ChannelTestFrequency)
		})
	}
	if config.BatchUpdateEnabled {
		logger.Logger.Info("batch update enabled with interval " + strconv.Itoa(config.BatchUpdateInterval) + "s")
		model.InitBatchUpdater()
	}
	if config.EnableMetric {
		logger.Logger.Info("metric enabled, will disable channel if too much request failed")
	}

	// Initialize monitoring
	if config.EnablePrometheusMetrics || config.OpenTelemetryEnabled {
		startTime := time.Unix(common.StartTime, 0)
		if err := monitor.InitMonitoring(common.Version, startTime.Format(time.RFC3339), runtime.Version(), startTime); err != nil {
			logger.Logger.Fatal("failed to initialize monitoring", zap.Error(err))
		}
		logger.Logger.Info("monitoring initialized")

		// Database query metrics are attached in model.InitDB when the handle
		// is opened, before any background worker can race the gorm callback
		// registration; nothing to do here.

		// Initialize Redis monitoring if enabled
		if common.IsRedisEnabled() {
			common.InitPrometheusRedisMonitoring()
		}
	}

	openai.InitTokenEncoders()
	client.Init()

	// Initialize global pricing manager
	relay.InitializeGlobalPricing()

	logLevel := glog.LevelInfo
	if config.DebugEnabled {
		logLevel = glog.LevelDebug
	}

	// Initialize HTTP server
	server := gin.New()
	server.RedirectTrailingSlash = false
	middlewares := []gin.HandlerFunc{
		gin.Recovery(),
	}

	if otelProviders != nil {
		middlewares = append(middlewares, otelgin.Middleware(config.OpenTelemetryServiceName))
	}

	middlewares = append(middlewares,
		gmw.NewLoggerMiddleware(
			gmw.WithLoggerMwColored(),
			gmw.WithLevel(logLevel.String()),
			gmw.WithLogger(logger.Logger.Named("gin")),
		),
	)
	server.Use(middlewares...)
	// This will cause SSE not to work!!!
	//server.Use(gzip.Gzip(gzip.DefaultCompression))
	server.Use(middleware.RequestId())
	server.Use(middleware.TracingMiddleware())

	// Add Prometheus middleware if enabled
	if config.EnablePrometheusMetrics {
		server.Use(middleware.PrometheusMiddleware())
		server.Use(middleware.PrometheusRateLimitMiddleware())
	}

	// middleware.SetUpLogger(server)

	// Initialize session store
	sessionSecret, err := base64.StdEncoding.DecodeString(config.SessionSecret)
	var sessionStore cookie.Store
	if err != nil {
		logger.Logger.Info("session secret is not base64 encoded, using raw value instead")
		sessionStore = cookie.NewStore([]byte(config.SessionSecret))
	} else {
		sessionStore = cookie.NewStore(sessionSecret, sessionSecret)
	}

	// Defaults to Secure=true (production-safe). Local HTTP development must
	// explicitly set ENABLE_COOKIE_SECURE=false.
	cookieSecure := config.EnableCookieSecure
	if !cookieSecure {
		logger.Logger.Warn("ENABLE_COOKIE_SECURE=false: session cookies will be sent over plain HTTP; do not use this configuration in production")
	}
	sessionStore.Options(sessions.Options{
		Path:     "/",
		MaxAge:   3600 * config.CookieMaxAgeHours,
		HttpOnly: true,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	server.Use(sessions.Sessions("session", sessionStore))

	// Add Prometheus metrics endpoint if enabled
	if config.EnablePrometheusMetrics {
		server.GET("/metrics", middleware.MetricsAuth(), gin.WrapH(promhttp.Handler()))
		logger.Logger.Info("Prometheus metrics endpoint available at /metrics")
	}

	router.SetRouter(server, buildFS)
	port := config.ServerPort
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}
	addr := ":" + port
	srv := &http.Server{Addr: addr, Handler: server}

	// Start the pprof profiling listener (separate from the API server) when enabled.
	var pprofSrv *http.Server
	if config.EnablePprof {
		pprofSrv = startPprofServer(config.PprofListen)
	}

	// Start server in background
	go func() {
		logger.Logger.Info("server started", zap.String("address", "http://localhost:"+port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Logger.Fatal("failed to start HTTP server", zap.Error(err))
		}
	}()

	// Handle shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Logger.Info("shutdown signal received, starting graceful drain")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(config.ShutdownTimeoutSec)*time.Second)
	defer cancel()

	runShutdownSequence(shutdownCtx, logger.Logger,
		newShutdownSequence(srv, pprofSrv, otelProviders, config.BatchUpdateEnabled, stopBackgroundWorkers))
}

// shutdownStep is one stage of the ordered graceful shutdown sequence.
//
// The sequence is data rather than straight-line code so the order itself can be
// asserted by a test without starting a server: admissions stop first, then every
// producer drains, only then are the consuming sinks closed, then the exporters
// are flushed, and the database is closed last.
type shutdownStep struct {
	// name identifies the step in deadline reports; keep it stable, it is a log field.
	name string
	// failureLog is the message logged when run reports an error.
	failureLog string
	// run performs the step. A nil error means the step completed.
	run func(ctx context.Context) error
}

// newShutdownSequence builds the ordered shutdown steps for the running process.
//
// It only assembles closures; nothing is executed until runShutdownSequence runs
// them, which is what lets a test assert the order in isolation.
//
// Order and the reason for it:
//  1. stop_admissions       — flip the draining flag so nothing new is accepted.
//  2. http_server           — drain in-flight HTTP handlers.
//  3. pprof_server          — drain the profiling listener.
//  4. batch_updater         — stop the quota batch updater and queue its final flush.
//  5. background_tasks      — join billing/refund and other critical producers.
//  6. retention_workers     — cancel the periodic background workers and join the
//     retention cleaners, so no sweep is still deleting rows when the sinks and
//     then the database close underneath it.
//  7. trace_sinks           — close the consuming sinks only once every producer
//     above has stopped emitting; closing them earlier makes traces emitted by a
//     draining background or billing task count as dropped_closed.
//  8. otel_providers        — flush the exporters that the sinks handed data to.
//  9. database              — close the handles everything above was writing through.
//
// Parameters:
//   - srv: the API HTTP server; must not be nil.
//   - pprofSrv: the pprof listener, or nil when pprof is disabled.
//   - otelProviders: the OpenTelemetry provider bundle, or nil when disabled.
//   - batchUpdateEnabled: whether the quota batch updater was started.
//   - stopBackgroundWorkers: cancels the context shared by the periodic
//     background workers; nil is tolerated so a test can build the sequence
//     without starting any.
//
// Return values:
//   - []shutdownStep: the steps in the order they must run.
func newShutdownSequence(
	srv *http.Server,
	pprofSrv *http.Server,
	otelProviders *telemetry.ProviderBundle,
	batchUpdateEnabled bool,
	stopBackgroundWorkers context.CancelFunc,
) []shutdownStep {
	return []shutdownStep{
		{
			name:       "stop_admissions",
			failureLog: "failed to stop admissions",
			run: func(context.Context) error {
				graceful.SetDraining()
				return nil
			},
		},
		{
			name:       "http_server",
			failureLog: "server shutdown error",
			run: func(ctx context.Context) error {
				if srv == nil {
					return nil
				}
				return errors.Wrap(srv.Shutdown(ctx), "shutdown http server")
			},
		},
		{
			name:       "pprof_server",
			failureLog: "pprof server shutdown error",
			run: func(ctx context.Context) error {
				// Shut down the pprof listener if it was started.
				if pprofSrv == nil {
					return nil
				}
				return errors.Wrap(pprofSrv.Shutdown(ctx), "shutdown pprof server")
			},
		},
		{
			name:       "batch_updater",
			failureLog: "batch updater shutdown did not complete",
			run: func(ctx context.Context) error {
				// Stop batch updater and flush pending changes before draining other
				// tasks. This is critical because the batch updater holds uncommitted
				// quota changes in memory. The final flush itself is a critical task,
				// so it is joined by the background_tasks step below.
				if !batchUpdateEnabled {
					return nil
				}
				model.StopBatchUpdater(ctx)
				return errors.Wrap(ctx.Err(), "stop batch updater")
			},
		},
		{
			name:       "background_tasks",
			failureLog: "graceful drain finished with timeout/error",
			run: func(ctx context.Context) error {
				// Drain critical background tasks (billing, refunds, etc.)
				return errors.Wrap(graceful.Drain(ctx), "drain background tasks")
			},
		},
		{
			name:       "retention_workers",
			failureLog: "retention workers did not stop before the deadline",
			run: func(ctx context.Context) error {
				// Cancel every periodic background worker, then JOIN the ones
				// that write to the database. Cancelling alone only asks a
				// retention sweep to stop at its next chunk boundary
				// (model.ChunkedDeleteWithStats checks ctx.Err() between
				// chunks); without the join the last in-flight DELETE would
				// race the trace-sink close and the database close below it,
				// and nothing would report that the sweep was unfinished.
				if stopBackgroundWorkers != nil {
					stopBackgroundWorkers()
				}
				return errors.Wrap(errors.Join(
					model.WaitForRetentionCleaners(ctx),
					model.WaitForBackgroundWorkers(ctx),
					controller.WaitForDashboardAggregateWork(ctx),
					logger.WaitForRetentionWorkers(ctx),
				), "stop retention workers")
			},
		},
		{
			name:       "trace_sinks",
			failureLog: "failed to flush trace sinks",
			run: func(ctx context.Context) error {
				// Flush and close the buffered trace sinks now that the HTTP handlers
				// and every background/billing producer have stopped, so the last batch
				// is written before the database handle is closed and no draining task
				// can still submit into a closed sink.
				return errors.Wrap(tracing.Shutdown(ctx), "shutdown trace sinks")
			},
		},
		{
			name:       "otel_providers",
			failureLog: "failed to shutdown OpenTelemetry",
			run: func(ctx context.Context) error {
				if otelProviders == nil {
					return nil
				}
				return errors.Wrap(otelProviders.Shutdown(ctx), "shutdown opentelemetry providers")
			},
		},
		{
			name:       "database",
			failureLog: "failed to close database",
			run: func(ctx context.Context) error {
				// Close DB after all drains, sink closes and exporter flushes complete.
				// CloseDB is idempotent, so this stays correct even if some other exit
				// path closed the handles already.
				//
				// Once the shared shutdown deadline has expired, an earlier producer or
				// sink may still be using the pools. Closing them here would violate the
				// ordering this sequence exists to enforce and race those goroutines.
				// Process exit will reclaim the handles; report the skipped explicit
				// close as unfinished work instead.
				if err := ctx.Err(); err != nil {
					return errors.Wrap(err, "skip database close while shutdown work may still be running")
				}
				return errors.Wrap(model.CloseDB(), "close database")
			},
		},
	}
}

// runShutdownSequence runs the steps in order and reports the work that did not
// finish before ctx's deadline.
//
// Every step runs even after the deadline expires: a step that is skipped leaks
// the resource it owns (the database handle above all), so the sequence degrades
// to best effort rather than stopping. Each step keeps its own failure message,
// and any step whose failure is attributable to an expired or cancelled context
// is named in a single summary record so the unfinished work is visible.
//
// Parameters:
//   - ctx: the shutdown deadline shared by every step.
//   - lg: logger used for per-step failures and the deadline summary.
//   - steps: the ordered steps, as built by newShutdownSequence.
//
// Return values:
//   - []string: names of the steps that did not finish before the deadline, in
//     execution order; empty when the sequence completed within the deadline.
func runShutdownSequence(ctx context.Context, lg glog.Logger, steps []shutdownStep) []string {
	start := time.Now().UTC()

	var unfinished []string
	for _, step := range steps {
		if step.run == nil {
			continue
		}

		err := step.run(ctx)
		if err == nil {
			continue
		}

		lg.Error(step.failureLog,
			zap.String("shutdown_step", step.name),
			zap.Error(err))

		// Attribute the failure to the deadline only when the context actually
		// expired or was cancelled; an ordinary failure is already logged above.
		if ctx.Err() != nil ||
			errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, context.Canceled) {
			unfinished = append(unfinished, step.name)
		}
	}

	if len(unfinished) > 0 {
		lg.Error("graceful shutdown deadline expired with unfinished work",
			zap.Strings("unfinished_steps", unfinished),
			zap.Duration("elapsed", time.Since(start)),
			zap.Error(ctx.Err()))
		return unfinished
	}

	lg.Info("graceful shutdown complete", zap.Duration("elapsed", time.Since(start)))
	return nil
}

// startPprofServer starts a dedicated HTTP listener that serves the Go
// net/http/pprof profiling endpoints. It is intentionally kept separate from
// the public API server so the profiling surface is never mixed into normal
// request routing. pprof has no built-in authentication, so the listener
// defaults to loopback (config.PprofListen) and should only be bound to a
// non-loopback address behind a firewall or auth proxy.
func startPprofServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", nhpprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", nhpprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", nhpprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", nhpprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", nhpprof.Trace)

	srv := &http.Server{Addr: addr, Handler: mux}

	if !isLoopbackListenAddr(addr) {
		logger.Logger.Warn("pprof listener is bound to a non-loopback address; "+
			"pprof has no authentication, ensure it is protected by a firewall or auth proxy",
			zap.String("address", addr))
	}

	go func() {
		logger.Logger.Info("pprof server started", zap.String("address", "http://"+addr+"/debug/pprof/"))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Logger.Error("pprof server stopped unexpectedly", zap.Error(err))
		}
	}()

	return srv
}

// isLoopbackListenAddr reports whether the given listen address is bound to the
// loopback interface (and is therefore not reachable from other hosts).
func isLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Fall back to a best-effort string check when the address is malformed.
		host = strings.TrimSpace(addr)
	}
	if host == "" {
		// An empty host (e.g. ":6060") binds to all interfaces.
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func isThemeValid() error {
	// Backward compatibility: redirect "default" to "modern"
	if config.Theme == "default" {
		logger.Logger.Warn("the 'default' theme has been removed, automatically switching to 'modern'")
		config.Theme = "modern"
	}

	if !config.ValidThemes[config.Theme] {
		return errors.Errorf("invalid theme: %s", config.Theme)
	}

	if config.Theme != "modern" {
		logger.Logger.Warn("recommend using the modern theme, as the other themes are no longer being actively maintained.")
	}

	return nil
}
