package model

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Laisky/one-api/common/logger"
)

// TestCreateTraceStoresValidUTF8URL covers a raw request line such as
// "GET /?a=\xc0": Go keeps RawQuery byte-for-byte, so URL.String() carries the
// invalid byte into the trace URL. MySQL (utf8mb4 strict) and PostgreSQL reject
// the INSERT and the OTLP span attribute poisons its export batch, so the
// stored URL and the span attribute must both be valid UTF-8.
func TestCreateTraceStoresValidUTF8URL(t *testing.T) {
	setupTestDatabase(t)
	require.NoError(t, DB.Exec("DELETE FROM traces WHERE trace_id = 'test-trace-utf8'").Error)

	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	ctx, span := provider.Tracer("test").Start(context.Background(), "req")
	ctx = oteltrace.ContextWithSpan(gmw.SetLogger(ctx, logger.Logger), span)

	rawURL := "/api/status?a=\xc0\xc1&b=ok"
	require.False(t, utf8.ValidString(rawURL))

	trace, err := CreateTrace(ctx, "test-trace-utf8", rawURL, "GET", 0)
	require.NoError(t, err)
	require.True(t, utf8.ValidString(trace.URL), "trace URL %q must be valid UTF-8", trace.URL)
	require.Equal(t, "/api/status?a=�&b=ok", trace.URL)

	var stored Trace
	require.NoError(t, DB.Where("trace_id = ?", "test-trace-utf8").First(&stored).Error)
	require.Equal(t, trace.URL, stored.URL)

	span.End()
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	var urlAttr attribute.Value
	for _, kv := range spans[0].Attributes {
		if kv.Key == "one_api.url" {
			urlAttr = kv.Value
		}
	}
	require.Equal(t, trace.URL, urlAttr.AsString())
}

// TestEnforceTraceURLLimitKeepsRuneBoundary pins that byte truncation never
// splits a multi-byte rune and always respects the byte budget the column is
// sized for.
func TestEnforceTraceURLLimitKeepsRuneBoundary(t *testing.T) {
	long := "/" + strings.Repeat("é", 2100) // 4201 bytes, 2101 runes
	require.Greater(t, len(long), maxTraceURLLength)

	got, truncated := enforceTraceURLLimit(long)
	require.True(t, truncated)
	require.True(t, utf8.ValidString(got), "truncated URL must stay valid UTF-8")
	require.LessOrEqual(t, len(got), maxTraceURLLength)

	veryLong := "/" + strings.Repeat("模", 5000) // 15001 bytes, 5001 runes
	got, truncated = enforceTraceURLLimit(veryLong)
	require.True(t, truncated)
	require.True(t, utf8.ValidString(got))
	require.LessOrEqual(t, len(got), maxTraceURLLength, "byte budget must hold for many-rune input too")

	ascii := "/" + strings.Repeat("a", 5000)
	got, truncated = enforceTraceURLLimit(ascii)
	require.True(t, truncated)
	require.Equal(t, maxTraceURLLength, len(got))
}
