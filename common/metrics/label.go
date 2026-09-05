package metrics

import (
	"strings"
	"unicode/utf8"
)

// SanitizeLabelValue returns value with every run of invalid UTF-8 bytes
// replaced by U+FFFD so the result is safe to use as a Prometheus label value
// or as an OTLP string attribute.
//
// Why: request paths, model names, channel names and similar strings that end
// up as metric labels are caller-controlled and may carry raw bytes (for
// example GET /%c0 decodes to "/\xc0"). prometheus/client_golang panics in
// WithLabelValues on such values, and the OTLP exporters fail to marshal the
// whole batch because protobuf string fields must be valid UTF-8; with
// cumulative metrics that failure repeats on every later export.
//
// Valid strings are returned unchanged without allocating.
func SanitizeLabelValue(value string) string {
	if utf8.ValidString(value) {
		return value
	}
	return strings.ToValidUTF8(value, "�")
}
