package telemetry

import (
	"net/url"
	"os"
	"strconv"
	"strings"

	laerrors "github.com/Laisky/errors/v2"
	"golang.org/x/net/http/httpguts"

	"github.com/Laisky/one-api/common/config"
)

// validateLogResourceEnvironment checks bounded resource syntax before any SDK
// detector can echo private malformed values through the global error handler.
// It does not rewrite the environment or expand the allowed resource identity.
func validateLogResourceEnvironment() error {
	raw := os.Getenv("OTEL_RESOURCE_ATTRIBUTES")
	if len(raw) > 64*1024 {
		return laerrors.New("invalid OTEL_RESOURCE_ATTRIBUTES syntax")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	for pair := range strings.SplitSeq(raw, ",") {
		key, value, found := strings.Cut(pair, "=")
		if !found || strings.TrimSpace(key) == "" {
			return laerrors.New("invalid OTEL_RESOURCE_ATTRIBUTES syntax")
		}
		if _, err := url.PathUnescape(strings.TrimSpace(value)); err != nil {
			return laerrors.New("invalid OTEL_RESOURCE_ATTRIBUTES syntax")
		}
	}
	return nil
}

// validateLogExporterEnvironment checks only environment converters consumed by
// the pinned log exporter. Explicit endpoint/compression settings and valid
// signal-over-global precedence remain unchanged; invalid diagnostics contain
// the fixed variable name, never its value or the SDK parser's wrapped cause.
func validateLogExporterEnvironment() error {
	endpointHasScheme := false
	if key, raw := firstLogExporterEnvironment("ENDPOINT"); raw != "" {
		if len(raw) > 64*1024 {
			return laerrors.Errorf("invalid %s syntax", key)
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return laerrors.Errorf("invalid %s syntax", key)
		}
		endpointHasScheme = parsed.Scheme != ""
	}
	if key, raw := firstLogExporterEnvironment("TIMEOUT"); raw != "" {
		if len(raw) > 64*1024 {
			return laerrors.Errorf("invalid %s syntax", key)
		}
		if _, err := strconv.Atoi(raw); err != nil {
			return laerrors.Errorf("invalid %s syntax", key)
		}
	}
	if key, raw := firstLogExporterEnvironment("HEADERS"); raw != "" {
		if len(raw) > 64*1024 {
			return laerrors.Errorf("invalid %s syntax", key)
		}
		for pair := range strings.SplitSeq(raw, ",") {
			name, value, found := strings.Cut(pair, "=")
			if !found || !httpguts.ValidHeaderFieldName(strings.TrimSpace(name)) {
				return laerrors.Errorf("invalid %s syntax", key)
			}
			if _, err := url.PathUnescape(value); err != nil {
				return laerrors.Errorf("invalid %s syntax", key)
			}
		}
	}
	// WithInsecure or an endpoint's nonempty scheme already resolves the SDK's
	// insecure setting, so unused environment values must not change acceptance.
	if !config.OpenTelemetryInsecure && !endpointHasScheme {
		if key, raw := firstLogExporterEnvironment("INSECURE"); raw != "" {
			if len(raw) > 64*1024 {
				return laerrors.Errorf("invalid %s syntax", key)
			}
			lower := strings.ToLower(raw)
			if lower != "true" && lower != "false" {
				return laerrors.Errorf("invalid %s syntax", key)
			}
		}
	}
	return nil
}

// firstLogExporterEnvironment returns the first nonempty signal/global setting,
// matching the pinned SDK's valid configuration precedence without mutation.
func firstLogExporterEnvironment(suffix string) (key, value string) {
	for _, prefix := range []string{"OTEL_EXPORTER_OTLP_LOGS_", "OTEL_EXPORTER_OTLP_"} {
		key = prefix + suffix
		if value = os.Getenv(key); value != "" {
			return key, value
		}
	}
	return "", ""
}
