package otelbridge

import (
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

// TestTimeOverflowPreservesLogAttribute reproduces the loss of entire OTLP
// records when a valid Zap time cannot fit into signed Unix nanoseconds.
func TestTimeOverflowPreservesLogAttribute(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value time.Time
	}{
		{name: "zero", value: time.Time{}},
		{name: "future", value: time.Date(2500, 1, 2, 3, 4, 5, 123456789, time.UTC)},
		{name: "historical_offset", value: time.Date(1800, 8, 7, 6, 5, 4, 987654321, time.FixedZone("historical", -5*3600))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger, output, _ := newTestBridge(t, zapcore.InfoLevel)
			err := logger.Core().Write(
				zapcore.Entry{Message: "time is application data", Level: zapcore.InfoLevel},
				[]zap.Field{zap.Time("at", tc.value), zap.Times("series", []time.Time{time.Unix(0, 0), tc.value})},
			)
			require.NoError(t, err)
			record := output.only(t)
			require.Equal(t, attribute.STRING, fidelityAttr(t, record, "at").Type())
			require.Equal(t, tc.value.Format(time.RFC3339Nano), fidelityAttr(t, record, "at").AsString())
			series := fidelityAttr(t, record, "series").AsSlice()
			require.Len(t, series, 2)
			require.Equal(t, attribute.INT64, series[0].Type())
			require.Equal(t, int64(0), series[0].AsInt64())
			require.Equal(t, attribute.STRING, series[1].Type())
			require.Equal(t, tc.value.Format(time.RFC3339Nano), series[1].AsString())
			require.Equal(t, "time is application data", record.Body().AsString())
		})
	}
}
