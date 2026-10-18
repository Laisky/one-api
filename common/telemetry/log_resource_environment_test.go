package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Laisky/one-api/common/config"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

// TestLogResourceEnvironmentRejectedBeforeSDK verifies malformed deployment
// resource syntax fails with a fixed diagnostic before SDK error handlers can
// echo private values. It does not mutate or repair the operator's environment.
func TestLogResourceEnvironmentRejectedBeforeSDK(t *testing.T) {
	clearLogExporterTestEnvironment(t)
	for _, raw := range []string{
		"host.name=SYNTHETIC_PRIVATE_VALUE%zz",
		"SYNTHETIC_PRIVATE_VALUE",
		"=SYNTHETIC_PRIVATE_VALUE",
	} {
		for _, constructor := range []string{"resource", "logs"} {
			t.Run(constructor+"/"+fmt.Sprint(len(raw)), func(t *testing.T) {
				t.Setenv("OTEL_RESOURCE_ATTRIBUTES", raw)
				old := otel.GetErrorHandler()
				var diagnostics []string
				otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { diagnostics = append(diagnostics, err.Error()) }))
				t.Cleanup(func() { otel.SetErrorHandler(old) })
				var err error
				if constructor == "resource" {
					_, err = buildResource(context.Background())
				} else {
					provider, createErr := newLoggerProvider(context.Background(), sdkresource.Empty())
					err = createErr
					if provider != nil {
						t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
					}
				}
				require.Empty(t, diagnostics, "validation must precede SDK parsing/error reporting")
				require.Error(t, err)
				require.Equal(t, "invalid OTEL_RESOURCE_ATTRIBUTES syntax", err.Error())
				require.Equal(t, raw, os.Getenv("OTEL_RESOURCE_ATTRIBUTES"))
			})
		}
	}
}

// TestLogExporterEnvironmentRejectedBeforeSDK covers the log exporter's concrete
// raw-value parser diagnostics without emitting logs or contacting a collector.
func TestLogExporterEnvironmentRejectedBeforeSDK(t *testing.T) {
	clearLogExporterTestEnvironment(t)
	for _, key := range []string{"OTEL_EXPORTER_OTLP_LOGS_TIMEOUT", "OTEL_EXPORTER_OTLP_TIMEOUT"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
			t.Setenv("OTEL_EXPORTER_OTLP_LOGS_TIMEOUT", "")
			t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "")
			t.Setenv(key, "SYNTHETIC_PRIVATE_TIMEOUT")
			old := otel.GetErrorHandler()
			var diagnostics []string
			otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) { diagnostics = append(diagnostics, err.Error()) }))
			t.Cleanup(func() { otel.SetErrorHandler(old) })
			provider, err := newLoggerProvider(context.Background(), sdkresource.Empty())
			if provider != nil {
				t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			}
			require.Empty(t, diagnostics, "raw invalid timeout must not reach the global handler")
			require.EqualError(t, err, "invalid "+key+" syntax")
			require.Equal(t, "SYNTHETIC_PRIVATE_TIMEOUT", os.Getenv(key))
		})
	}
}

// TestLogEnvironmentSyntaxValidationPreservesValidConfiguration verifies valid
// percent values, signal precedence, integer timeout semantics and ignored
// insecure settings retain their existing SDK interpretation without mutation.
func TestLogEnvironmentSyntaxValidationPreservesValidConfiguration(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "host.name=worker%20one,service.name=one-api")
	require.NoError(t, validateLogResourceEnvironment())
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "https://collector.example/v1/logs")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "SYNTHETIC_UNUSED%zz")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_TIMEOUT", "-1")
	t.Setenv("OTEL_EXPORTER_OTLP_TIMEOUT", "SYNTHETIC_UNUSED_TIMEOUT")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_HEADERS", "X-Test=value%20one,Authorization=Bearer%20SYNTHETIC_FAKE")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_INSECURE", "SYNTHETIC_UNUSED_BOOL")
	require.NoError(t, validateLogExporterEnvironment())
	require.Equal(t, "-1", os.Getenv("OTEL_EXPORTER_OTLP_LOGS_TIMEOUT"))
	require.Equal(t, "host.name=worker%20one,service.name=one-api", os.Getenv("OTEL_RESOURCE_ATTRIBUTES"))
	for _, tc := range []struct{ key, value string }{
		{"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "https://collector.example/SYNTHETIC%zz"},
		{"OTEL_EXPORTER_OTLP_LOGS_HEADERS", "SYNTHETIC_PRIVATE_HEADER"},
		{"OTEL_EXPORTER_OTLP_LOGS_HEADERS", "X-Test=SYNTHETIC%zz"},
		{"OTEL_EXPORTER_OTLP_LOGS_TIMEOUT", strings.Repeat("1", 64*1024+1)},
	} {
		t.Run(tc.key+"/"+fmt.Sprint(len(tc.value)), func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			require.EqualError(t, validateLogExporterEnvironment(), "invalid "+tc.key+" syntax")
		})
	}
	oldInsecure := config.OpenTelemetryInsecure
	config.OpenTelemetryInsecure = false
	t.Cleanup(func() { config.OpenTelemetryInsecure = oldInsecure })
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	require.EqualError(t, validateLogExporterEnvironment(), "invalid OTEL_EXPORTER_OTLP_LOGS_INSECURE syntax")
	config.OpenTelemetryInsecure = true
	require.NoError(t, validateLogExporterEnvironment(), "explicit WithInsecure skips SDK env parsing")
}

// clearLogExporterTestEnvironment isolates constructor fixtures from inherited
// transport settings, including certificate-file paths; tests never inspect them.
func clearLogExporterTestEnvironment(t *testing.T) {
	t.Helper()
	for _, prefix := range []string{"OTEL_EXPORTER_OTLP_LOGS_", "OTEL_EXPORTER_OTLP_"} {
		for _, suffix := range []string{"TIMEOUT", "ENDPOINT", "HEADERS", "INSECURE", "COMPRESSION", "CERTIFICATE", "CLIENT_CERTIFICATE", "CLIENT_KEY"} {
			t.Setenv(prefix+suffix, "")
		}
	}
}
