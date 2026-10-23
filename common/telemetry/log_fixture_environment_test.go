package telemetry

import "testing"

// clearLogExporterTestEnvironment isolates synthetic local HTTP fixtures from
// inherited transport settings, without printing or inspecting their values.
func clearLogExporterTestEnvironment(t *testing.T) {
	t.Helper()
	for _, prefix := range []string{"OTEL_EXPORTER_OTLP_LOGS_", "OTEL_EXPORTER_OTLP_"} {
		for _, suffix := range []string{"TIMEOUT", "ENDPOINT", "HEADERS", "INSECURE", "COMPRESSION", "CERTIFICATE", "CLIENT_CERTIFICATE", "CLIENT_KEY"} {
			t.Setenv(prefix+suffix, "")
		}
	}
}
