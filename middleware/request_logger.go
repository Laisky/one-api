package middleware

import (
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// RequestLogger returns the per-request logger middleware exactly as the server
// installs it.
//
// It exists so the server and the tests that prove request-scoped logs reach
// every configured sink build the same middleware. Tests that rebuilt it by hand
// drifted from production once already: they bound no process "host" field, so
// they never saw the field collision that kept every request log out of the
// OTLP export.
//
// It reads logger.Logger when called, so it must run after
// logger.SetupEnhancedLogger has installed the final process logger.
//
// Parameters: none.
//
// Return values:
//   - gin.HandlerFunc: the gin-middlewares logger middleware.
func RequestLogger() gin.HandlerFunc {
	level := glog.LevelInfo
	if config.DebugEnabled {
		level = glog.LevelDebug
	}

	return gmw.NewLoggerMiddleware(
		gmw.WithLoggerMwColored(),
		gmw.WithLevel(level.String()),
		gmw.WithLogger(logger.Logger.Named("gin")),
	)
}
