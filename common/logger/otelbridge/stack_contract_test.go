package otelbridge

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"

	stackerrors "github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// stackContractOrigin creates an error with a distinguishable origin stack for the logging tests.
func stackContractOrigin() error {
	return stackerrors.WithStack(stderrors.New("synthetic validation failure"))
}

// TestLogEncodingMarkerRejectsWithoutLeaking verifies the fixed marker and whole-record rejection.
func TestLogEncodingMarkerRejectsWithoutLeaking(t *testing.T) {
	const sensitive = "synthetic-private-value"
	invalid := sensitive + string([]byte{0xff})
	err := ValidateLogValue(attribute.StringValue(invalid))
	require.True(t, err == ErrLogEncoding)
	require.EqualError(t, err, "application log cannot be encoded faithfully")
	require.NotContains(t, fmt.Sprintf("%+v", err), sensitive)

	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	calls := 0
	err = lg.Core().Write(zapcore.Entry{Level: zapcore.InfoLevel, Message: "reject"}, []zap.Field{
		zap.String("sensitive", sensitive), zap.Reflect("unsupported", make(chan int)),
		zap.Stringer("unnecessary", countingLogStringer{&calls}),
	})
	require.True(t, err == ErrLogEncoding)
	require.EqualError(t, err, "application log cannot be encoded faithfully")
	require.NotContains(t, fmt.Sprintf("%+v", err), sensitive)
	require.Zero(t, calls)
	require.Empty(t, out.all())
}

// TestLogStackContract verifies stack-bearing errors use Zap's verbose field and pinned SDK type projection.
func TestLogStackContract(t *testing.T) {
	origin := stackContractOrigin()
	lg, out, _ := newTestBridge(t, zapcore.InfoLevel)
	require.NoError(t, lg.Core().Write(zapcore.Entry{Level: zapcore.ErrorLevel, Message: "validation", Stack: "logger stack"}, []zap.Field{
		zap.Error(origin), zap.String("exception.type", "explicit"), zap.String("exception.stacktrace", "explicit trace"),
	}))
	r := out.only(t)
	require.Equal(t, "synthetic validation failure", fidelityAttr(t, r, "exception.message").AsString())
	require.Equal(t, "explicit", fidelityAttr(t, r, "exception.type").AsString())
	require.Equal(t, "explicit trace", fidelityAttr(t, r, "exception.stacktrace").AsString())
	require.Contains(t, fidelityAttr(t, r, "errorVerbose").AsString(), "stackContractOrigin")

	attrs, err := DeriveLogExceptionAttributes(stderrors.New("synthetic validation failure"), nil, nil)
	require.NoError(t, err)
	require.Equal(t, "*errors.errorString", attrs[1].Value.AsString())
	attrs, err = DeriveLogExceptionAttributes(origin, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "*errors.withStack", attrs[1].Value.AsString())
	attrs, err = DeriveLogExceptionAttributes(fmt.Errorf("part: %w", origin), nil, nil)
	require.NoError(t, err)
	require.Equal(t, "*errors.withStack", attrs[1].Value.AsString())
}

// TestPrivacyCoreDropsStackErrors verifies stack and exception fields never enter privacy-mode export.
func TestPrivacyCoreDropsStackErrors(t *testing.T) {
	exporter := &recordingExporter{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	holder := NewProviderHolder()
	require.NoError(t, holder.Install(provider))
	lg := zap.New(NewPrivacyCore(holder, DefaultScopeName, zapcore.InfoLevel))
	require.NoError(t, lg.Core().Write(zapcore.Entry{Level: zapcore.ErrorLevel, Message: "sensitive", Stack: "sensitive logger stack"}, []zap.Field{
		zap.Error(stackContractOrigin()), zap.String("exception.stacktrace", "sensitive trace"), zap.Int("status_code", 500),
	}))
	record := exporter.only(t)
	require.Equal(t, "application log", record.Body().AsString())
	_, found := attrOf(record, "errorVerbose")
	require.False(t, found)
	_, found = attrOf(record, "exception.stacktrace")
	require.False(t, found)
	_, found = attrOf(record, "exception.type")
	require.False(t, found)
	require.EqualValues(t, 500, fidelityAttr(t, record, "status_code").AsInt64())
}
