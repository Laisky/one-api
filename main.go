package main

import (
	"context"
	"embed"
	"encoding/base64"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
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

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/telemetry"
	"github.com/songquanpeng/one-api/controller"
	"github.com/songquanpeng/one-api/middleware"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/monitor"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/router"
)

//go:embed web/build/*

var buildFS embed.FS

func main() {
	ctx := context.Background()

	common.Init()
	logger.SetupLogger()
	logger.StartLogRetentionCleaner(ctx, config.LogRetentionDays, logger.LogDir)

	// Setup enhanced logger with alertPusher integration
	logger.SetupEnhancedLogger(ctx)

	var (
		err           error
		otelProviders *telemetry.ProviderBundle
	)

	logger.Logger.Info("One API started", zap.String("version", common.Version))

	if config.GinMode != gin.DebugMode {
		gin.SetMode(gin.ReleaseMode)
	}

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

	// Initialize SQL Database
	model.InitDB()
	model.InitLogDB()
	model.StartTraceRetentionCleaner(ctx, config.TraceRetentionDays)
	model.StartAsyncTaskRetentionCleaner(ctx, config.AsyncTaskRetentionDays)
	err = model.CreateRootAccountIfNeed()
	if err != nil {
		logger.Logger.Fatal("database init error", zap.Error(err))
	}
	defer func() {
		err := model.CloseDB()
		if err != nil {
			logger.Logger.Fatal("failed to close database", zap.Error(err))
		}
	}()

	// Initialize Redis
	err = common.InitRedisClient()
	if err != nil {
		logger.Logger.Fatal("failed to initialize Redis", zap.Error(err))
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
		go model.SyncOptions(config.SyncFrequency)
		go model.SyncChannelCache(config.SyncFrequency)
	}
	if config.ChannelTestFrequency > 0 {
		go controller.AutomaticallyTestChannels(config.ChannelTestFrequency)
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

		// Initialize database monitoring
		if err := model.InitPrometheusDBMonitoring(); err != nil {
			logger.Logger.Fatal("failed to initialize database monitoring", zap.Error(err))
		}

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

	cookieSecure := false
	if config.EnableCookieSecure {
		cookieSecure = true
	} else {
		logger.Logger.Warn("ENABLE_COOKIE_SECURE is not set, using insecure cookie store")
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
		server.GET("/metrics", middleware.AdminAuth(), gin.WrapH(promhttp.Handler()))
		logger.Logger.Info("Prometheus metrics endpoint available at /metrics")
	}

	router.SetRouter(server, buildFS)
	port := config.ServerPort
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}
	addr := ":" + port
	srv := &http.Server{Addr: addr, Handler: server}

	// Start server in background
	go func() {
		logger.Logger.Info("server started", zap.String("address", "http://localhost:"+port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Logger.Fatal("failed to start HTTP server", zap.Error(err))
		}
	}()

	// Handle shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Logger.Info("shutdown signal received, starting graceful drain")
	graceful.SetDraining()

	// Stop accepting new requests and wait for handlers to return
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(config.ShutdownTimeoutSec)*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Logger.Error("server shutdown error", zap.Error(err))
	}

	// Stop batch updater and flush pending changes before draining other tasks.
	// This is critical because batch updater holds uncommitted quota changes in memory.
	if config.BatchUpdateEnabled {
		model.StopBatchUpdater(shutdownCtx)
	}

	// Drain critical background tasks (billing, refunds, etc.)
	if err := graceful.Drain(shutdownCtx); err != nil {
		logger.Logger.Error("graceful drain finished with timeout/error", zap.Error(err))
	}

	if otelProviders != nil {
		if err := otelProviders.Shutdown(shutdownCtx); err != nil {
			logger.Logger.Error("failed to shutdown OpenTelemetry", zap.Error(err))
		}
	}

	// Close DB after all drains complete
	if derr := model.CloseDB(); derr != nil {
		logger.Logger.Error("failed to close database", zap.Error(derr))
	}
}

func isThemeValid() error {
	if !config.ValidThemes[config.Theme] {
		return errors.Errorf("invalid theme: %s", config.Theme)
	}

	if config.Theme != "modern" {
		logger.Logger.Warn("recommend using the default modern theme, as the other themes are no longer being actively maintained.")
	}

	return nil
}
