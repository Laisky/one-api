package prometheus

import (
	"github.com/Laisky/one-api/common/metrics"
)

// labelValues returns vals with every element made valid UTF-8 (see
// metrics.SanitizeLabelValue) so that prometheus WithLabelValues, which panics
// on an invalid label value, can never take a request down because a caller
// passed a raw request path, model name or similar caller-controlled string.
// Every WithLabelValues call in this package must go through it.
func labelValues(vals ...string) []string {
	for i, v := range vals {
		vals[i] = metrics.SanitizeLabelValue(v)
	}
	return vals
}
