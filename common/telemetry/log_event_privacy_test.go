package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"

	"github.com/Laisky/one-api/common/logger/otelbridge"
	"github.com/Laisky/one-api/common/metrics"
)

// eventPrivacyCapture observes actual SDK processor enablement and admission.
type eventPrivacyCapture struct {
	captureProcessor
	count        int
	enabledEvent string
}

// Enabled records the event name reaching the downstream processor.
func (p *eventPrivacyCapture) Enabled(_ context.Context, param sdklog.EnabledParameters) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabledEvent = param.EventName
	return true
}

// OnEmit records admitted events without performing network I/O.
func (p *eventPrivacyCapture) OnEmit(_ context.Context, record *sdklog.Record) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count++
	p.last = record.Clone()
	return nil
}

// TestSDKEventNameSurvivesPrivacyAndBudget covers the real EventName protocol
// field, independently of an event.name attribute, through privacy and the gate.
func TestSDKEventNameSurvivesPrivacyAndBudget(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	for _, tc := range []struct {
		name, event, expected string
		budget                int64
		admitted              bool
	}{
		{"benign", "cache.refresh.completed", "cache.refresh.completed", 1 << 20, true},
		{"secret", "cache.refresh token=SYNTHETIC_PRIVATE_EVENT", "cache.refresh token=[REDACTED]", 1 << 20, true},
		{"byte budget", strings.Repeat("event", 256), "", 600, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spy := installLogExportSpy(t)
			capture := &eventPrivacyCapture{}
			gate := newBoundedProcessor(capture, 10, tc.budget)
			resource := sdkresource.Empty()
			provider := sdklog.NewLoggerProvider(sdklog.WithResource(resource),
				sdklog.WithProcessor(&privacyLogProcessor{next: gate, resource: resource}))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			lg := provider.Logger(otelbridge.DefaultScopeName)
			require.True(t, lg.Enabled(context.Background(), otellog.EnabledParameters{EventName: tc.event}))
			var record otellog.Record
			record.SetBody(attribute.StringValue("worker completed"))
			record.SetEventName(tc.event)
			record.AddAttributes(attribute.String("event.name", "attribute-only"))
			lg.Emit(context.Background(), record)
			capture.mu.Lock()
			count, last, enabled := capture.count, capture.last.Clone(), capture.enabledEvent
			capture.mu.Unlock()
			if !tc.admitted {
				require.Zero(t, count, "the EventName bytes must count before queue admission")
				require.Zero(t, gate.residentBytes.Load())
				require.Zero(t, gate.residentRecords.Load())
				require.Equal(t, 1, spy.outcome(metrics.AppLogExportOutcomeDroppedQueueFull))
				return
			}
			require.Equal(t, 1, count)
			require.Equal(t, tc.expected, enabled)
			require.Equal(t, tc.expected, last.EventName())
			require.Equal(t, "worker completed", last.Body().AsString())
			require.NotContains(t, last.EventName(), "SYNTHETIC_PRIVATE")
		})
	}
}

// TestSDKEventNameSanitizerBound checks oversized native event text is bounded
// before the downstream queue and cannot make a benign body disappear.
func TestSDKEventNameSanitizerBound(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	capture := &eventPrivacyCapture{}
	gate := newBoundedProcessor(capture, 10, 1<<20)
	resource := sdkresource.Empty()
	provider := sdklog.NewLoggerProvider(sdklog.WithResource(resource),
		sdklog.WithProcessor(&privacyLogProcessor{next: gate, resource: resource}))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	var record otellog.Record
	record.SetBody(attribute.StringValue("worker completed"))
	record.SetEventName(strings.Repeat("cache.event.", 1000))
	provider.Logger(otelbridge.DefaultScopeName).Emit(context.Background(), record)
	capture.mu.Lock()
	defer capture.mu.Unlock()
	require.Equal(t, 1, capture.count)
	require.NotEmpty(t, capture.last.EventName())
	require.LessOrEqual(t, len(capture.last.EventName()), 4096+len(" [truncated]"))
	require.Contains(t, capture.last.EventName(), "[truncated]")
}

// TestLabeledReadPayloadAndTypedErrorContext proves generic message/string text
// cannot claim read-operation provenance, while actual wrapped error causes remain
// useful and their own labeled payloads/credentials are still sanitized.
func TestLabeledReadPayloadAndTypedErrorContext(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	exporter := &operationalLogExporter{}
	resource := sdkresource.Empty()
	provider := sdklog.NewLoggerProvider(sdklog.WithResource(resource),
		sdklog.WithProcessor(&privacyLogProcessor{next: sdklog.NewSimpleProcessor(exporter), resource: resource}))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(provider))
	lg := zap.New(otelbridge.NewPrivacyCore(holder, otelbridge.DefaultScopeName, zapcore.InfoLevel))
	cases := []struct {
		message string
		cause   error
		useful  string
	}{
		{"read body: SYNTHETIC_PRIVATE_BODY", nil, ""},
		{"reading payload: SYNTHETIC_PRIVATE_BODY", nil, ""},
		{"transport failed", errors.New("read body: SYNTHETIC_PRIVATE_BODY"), ""},
		{"transport failed", fmt.Errorf("read body: %w", io.ErrUnexpectedEOF), "unexpected EOF"},
		{"transport failed", fmt.Errorf("read response body: %w", io.ErrClosedPipe), "io: read/write on closed pipe"},
		{"transport failed", fmt.Errorf("read body: %w", errors.New("response body: SYNTHETIC_PRIVATE_CAUSE")), ""},
		{"transport failed", fmt.Errorf("read body: %w", errors.New("password=SYNTHETIC_PRIVATE_CREDENTIAL")), ""},
	}
	for _, tc := range cases {
		fields := []zap.Field{zap.String("diagnostic", "read body: SYNTHETIC_PRIVATE_ATTRIBUTE")}
		if tc.cause != nil {
			fields = append(fields, zap.Error(tc.cause))
		}
		lg.Info(tc.message, fields...)
	}
	exporter.mu.Lock()
	records := append([]sdklog.Record(nil), exporter.records...)
	exporter.mu.Unlock()
	require.Len(t, records, len(cases))
	for i, record := range records {
		require.NotContains(t, record.Body().AsString(), "SYNTHETIC_PRIVATE")
		attrs := map[string]attribute.Value{}
		record.WalkAttributes(func(kv attribute.KeyValue) bool {
			attrs[string(kv.Key)] = kv.Value
			if kv.Value.Type() == attribute.STRING {
				require.NotContains(t, kv.Value.AsString(), "SYNTHETIC_PRIVATE")
			}
			return true
		})
		if cases[i].useful != "" {
			require.Contains(t, attrs["error"].AsString(), cases[i].useful)
		}
	}
}
