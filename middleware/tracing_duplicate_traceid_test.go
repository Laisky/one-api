package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Laisky/one-api/common/logger"
)

func TestTracingMiddleware_AllowsSameOTelTraceIDAcrossRequestsWithoutDB(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Ensure otelgin creates spans and honors traceparent headers.
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer func() {
		_ = tp.Shutdown(context.Background())
	}()

	engine := gin.New()
	engine.Use(otelgin.Middleware("one-api-test"))
	engine.Use(gmw.NewLoggerMiddleware(
		gmw.WithLevel(glog.LevelDebug.String()),
		gmw.WithLogger(logger.Logger.Named("gin-test")),
	))
	engine.Use(TracingMiddleware())
	engine.GET("/", func(c *gin.Context) { c.String(200, "ok") })

	// Two separate HTTP requests with the same distributed trace id.
	// traceparent format: version-traceid-spanid-flags
	// Use a fixed trace id so both requests share it, but otelgin will mint new server spans.
	const traceparent = "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"

	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("traceparent", traceparent)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

}
