package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestSanitizeAttributesRewritesOnlyInvalidStrings pins the processor's
// contract: valid strings, non-string values and empty input produce no
// replacements; invalid UTF-8 strings and string slices are rewritten with
// U+FFFD in place of every invalid byte run.
func TestSanitizeAttributesRewritesOnlyInvalidStrings(t *testing.T) {
	t.Parallel()

	require.Nil(t, sanitizeAttributes(nil))
	require.Nil(t, sanitizeAttributes([]attribute.KeyValue{
		attribute.String("url.path", "/v1/chat/completions"),
		attribute.String("unicode", "/模型/é"),
		attribute.Int("http.response.status_code", 200),
		attribute.Bool("ok", true),
		attribute.StringSlice("list", []string{"a", "b"}),
	}))

	fixed := sanitizeAttributes([]attribute.KeyValue{
		attribute.String("url.path", "/\xc0"),
		attribute.String("fine", "/fine"),
		attribute.StringSlice("mixed", []string{"ok", "bad\xff\xfe", "ok2"}),
	})
	require.Len(t, fixed, 2)
	require.Equal(t, attribute.String("url.path", "/�"), fixed[0])
	require.Equal(t, attribute.StringSlice("mixed", []string{"ok", "bad�", "ok2"}), fixed[1])
}

// TestUTF8AttributeSanitizerRewritesSpanAttributes verifies the processor
// rewrites start-time attributes on a real SDK span (the mechanism otelgin
// relies on) while leaving valid attributes and later attributes untouched.
func TestUTF8AttributeSanitizerRewritesSpanAttributes(t *testing.T) {
	t.Parallel()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(newUTF8AttributeSanitizer()),
		sdktrace.WithSyncer(exporter),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	_, span := provider.Tracer("test").Start(context.Background(), "req",
		sdktraceStartAttributes(
			attribute.String("url.path", "/\xc0"),
			attribute.String("http.request.method", "GET"),
		))
	span.SetAttributes(attribute.Int("http.response.status_code", 200))
	span.End()

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	got := map[attribute.Key]attribute.Value{}
	for _, kv := range spans[0].Attributes {
		got[kv.Key] = kv.Value
	}
	require.Equal(t, "/�", got["url.path"].AsString())
	require.Equal(t, "GET", got["http.request.method"].AsString())
	require.Equal(t, int64(200), got["http.response.status_code"].AsInt64())
	for _, kv := range spans[0].Attributes {
		if kv.Value.Type() == attribute.STRING {
			require.True(t, utf8.ValidString(kv.Value.AsString()), "attribute %s", kv.Key)
		}
	}
}

// TestNewTracerProviderExportsNonUTF8RequestPath is the end-to-end regression
// for the production incident on the tracing side: GET /%c0 through the real
// otelgin middleware produces a server span whose url.path attribute is the
// raw byte 0xC0. Without the sanitizer the OTLP HTTP exporter fails to marshal
// the batch ("string field contains invalid UTF-8") and every span in it is
// dropped; with newTracerProvider the batch reaches the collector.
func TestNewTracerProviderExportsNonUTF8RequestPath(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	var received atomic.Int64
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)

	ctx := context.Background()
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(strings.TrimPrefix(collector.URL, "http://")),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}),
	)
	require.NoError(t, err)

	provider := newTracerProvider(exporter, nil)
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })

	router := gin.New()
	router.Use(otelgin.Middleware("one-api-test", otelgin.WithTracerProvider(provider)))
	router.NoRoute(func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, rawPath := range []string{"/healthy", "/%c0", "/backup/%ff/.env"} {
		req := httptest.NewRequest(http.MethodGet, rawPath, nil)
		req.Header.Set("User-Agent", "scanner/1.0 \xc0\xc1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	}

	require.NoError(t, provider.ForceFlush(ctx), "a batch containing the /%%c0 span must still export")
	require.Equal(t, int64(1), received.Load(), "collector must receive the batch instead of it being dropped")
}

// TestTracerProviderWithoutSanitizerDropsBatch documents the failure mode the
// sanitizer exists for, so a future refactor that removes the processor from
// newTracerProvider is caught by the test above rather than in production.
func TestTracerProviderWithoutSanitizerDropsBatch(t *testing.T) {
	t.Parallel()

	var received atomic.Int64
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)

	ctx := context.Background()
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(strings.TrimPrefix(collector.URL, "http://")),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}),
	)
	require.NoError(t, err)

	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })

	_, span := provider.Tracer("test").Start(ctx, "req", sdktraceStartAttributes(attribute.String("url.path", "/\xc0")))
	span.End()

	require.Error(t, provider.ForceFlush(ctx))
	require.Equal(t, int64(0), received.Load())
}
