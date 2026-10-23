package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"

	"github.com/Laisky/one-api/common/logger/otelbridge"
)

// resolveNativeLogError derives metadata before the SDK can normalize it. Clone
// before adding attributes because Record's spare backing capacity may be shared
// with the caller. Clearing the copied Err prevents a second SDK callback.
func resolveNativeLogError(r otellog.Record, attrs []attribute.KeyValue) (otellog.Record, error) {
	derived, err := otelbridge.DeriveLogExceptionAttributes(r.Err(), attrs, nil)
	if err != nil {
		return otellog.Record{}, err
	}
	if len(derived) > 0 {
		r = r.Clone()
		r.AddAttributes(derived...)
	}
	r.SetErr(nil)
	return r, nil
}
