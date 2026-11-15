package otelbridge

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

// normalizeCases are attribute lists covering every place a key can repeat.
// The explicit expected map case also pins the nested-map replacement behavior
// now implemented correctly by the pinned SDK (sdk/log v1.47.0).
var normalizeCases = []struct {
	name     string
	attrs    []attribute.KeyValue
	repeated bool
	want     []attribute.KeyValue
}{
	{"unique", []attribute.KeyValue{attribute.String("a", "1"), attribute.Int("b", 2)}, false, nil},
	{"top-level repeat keeps first position", []attribute.KeyValue{
		attribute.String("host", "container"), attribute.String("url", "/v1"), attribute.String("host", "oneapi.example.com"),
	}, true, nil},
	{"three occurrences", []attribute.KeyValue{
		attribute.Int("n", 1), attribute.Int("m", 0), attribute.Int("n", 2), attribute.Int("n", 3),
	}, true, nil},
	{"repeat inside a map", []attribute.KeyValue{
		attribute.Map("worker", attribute.Int("attempt", 1), attribute.Int("attempt", 2)),
	}, true, nil},
	{"repeat inside a map inside a list", []attribute.KeyValue{
		attribute.Slice("items", attribute.StringValue("x"), attribute.MapValue(attribute.Int("k", 1), attribute.Int("k", 2))),
	}, true, nil},
	{"repeated key whose values are maps", []attribute.KeyValue{
		attribute.Map("m", attribute.Int("a", 1)), attribute.Map("m", attribute.Int("b", 2), attribute.Int("b", 3)),
	}, true, []attribute.KeyValue{attribute.Map("m", attribute.Int("b", 3))}},
	{"same key at different depths is not a repeat", []attribute.KeyValue{
		attribute.Int("k", 1), attribute.Map("m", attribute.Int("k", 2)),
	}, false, nil},
}

// hasNestedRepeat reports whether any map inside attrs holds a key twice.
//
// Parameters:
//   - attrs: the attributes to inspect.
//
// Return values:
//   - bool: whether a nested map repeats a key.
func hasNestedRepeat(attrs []attribute.KeyValue) bool {
	for _, kv := range attrs {
		if kv.Value.Type() != attribute.MAP {
			continue
		}
		nested := kv.Value.AsMap()
		seen := map[attribute.Key]bool{}
		for _, inner := range nested {
			if seen[inner.Key] {
				return true
			}
			seen[inner.Key] = true
		}
		if hasNestedRepeat(nested) {
			return true
		}
	}
	return false
}

// sdkRecorder keeps the records an SDK provider exports.
type sdkRecorder struct{ records []sdklog.Record }

// OnEmit implements sdklog.Processor by keeping a clone of each record.
func (r *sdkRecorder) OnEmit(_ context.Context, record *sdklog.Record) error {
	r.records = append(r.records, record.Clone())
	return nil
}

// Enabled implements sdklog.Processor.
func (r *sdkRecorder) Enabled(context.Context, sdklog.EnabledParameters) bool { return true }

// Shutdown implements sdklog.Processor.
func (r *sdkRecorder) Shutdown(context.Context) error { return nil }

// ForceFlush implements sdklog.Processor.
func (r *sdkRecorder) ForceFlush(context.Context) error { return nil }

// TestNormalizeMatchesSDKDeduplication pins NormalizeLogRecordAttributes and
// NormalizeLogValue to the OpenTelemetry SDK's own deduplication: for every
// case, the attributes and the map body they produce must be identical --
// keys, values and order -- to what the SDK produces with deduplication on.
//
// The bridge resolves collisions itself only because the SDK's per-attribute
// deduplication is measurably slower; this test is what lets it claim the
// standard behavior rather than an approximation of it.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestNormalizeMatchesSDKDeduplication(t *testing.T) {
	recorder := &sdkRecorder{}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(sdkresource.Empty()),
		sdklog.WithProcessor(recorder),
		sdklog.WithAttributeCountLimit(-1),
		sdklog.WithAttributeValueLengthLimit(-1),
	)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	sdk := provider.Logger("normalize-differential")

	for _, tc := range normalizeCases {
		t.Run(tc.name, func(t *testing.T) {
			recorder.records = nil
			var record otellog.Record
			record.SetBody(attribute.MapValue(tc.attrs...))
			record.AddAttributes(tc.attrs...)
			sdk.Emit(context.Background(), record)
			require.Len(t, recorder.records, 1)
			want := recorder.records[0]

			var wantAttrs []attribute.KeyValue
			want.WalkAttributes(func(kv attribute.KeyValue) bool { wantAttrs = append(wantAttrs, kv); return true })

			gotAttrs, repeated, err := NormalizeLogRecordAttributes(tc.attrs)
			require.NoError(t, err)
			require.Equal(t, tc.repeated, repeated)
			require.False(t, hasNestedRepeat(gotAttrs), "the bridge must never export a repeated nested key")
			require.Equal(t, wantAttrs, gotAttrs)
			if tc.want != nil {
				require.Equal(t, tc.want, gotAttrs)
			}

			gotBody, bodyRepeated, err := NormalizeLogValue(attribute.MapValue(tc.attrs...))
			require.NoError(t, err)
			require.Equal(t, tc.repeated, bodyRepeated)
			require.Equal(t, want.Body(), gotBody)
		})
	}
}

// TestNormalizeLeavesUniqueInputUntouched proves the common path costs no copy:
// a collision-free list is returned as the very same slice.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestNormalizeLeavesUniqueInputUntouched(t *testing.T) {
	attrs := []attribute.KeyValue{
		attribute.String("host", "container"),
		attribute.Map("worker", attribute.Int("attempt", 1)),
		attribute.Slice("items", attribute.MapValue(attribute.Int("k", 1))),
	}

	got, repeated, err := NormalizeLogRecordAttributes(attrs)
	require.NoError(t, err)
	require.False(t, repeated)
	require.Same(t, &attrs[0], &got[0], "unique input must be returned without copying")
}

// TestNormalizeStillRejectsWhatCannotBeEncoded proves that resolving
// collisions did not loosen any other fidelity rule of the strict validator.
//
// Parameters:
//   - t: the test handle.
//
// Return values: none.
func TestNormalizeStillRejectsWhatCannotBeEncoded(t *testing.T) {
	deep := attribute.StringValue("leaf")
	for range maxLogEncodingDepth + 1 {
		deep = attribute.MapValue(attribute.KeyValue{Key: "next", Value: deep})
	}
	invalid := string([]byte{255})

	for _, tc := range []struct {
		name  string
		attrs []attribute.KeyValue
	}{
		{"empty top-level key", []attribute.KeyValue{{Key: "", Value: attribute.IntValue(1)}}},
		{"invalid UTF-8 key", []attribute.KeyValue{attribute.Int(invalid, 1)}},
		{"invalid UTF-8 value", []attribute.KeyValue{attribute.String("k", invalid)}},
		{"invalid UTF-8 value after a repeat", []attribute.KeyValue{
			attribute.String("k", "ok"), attribute.String("k", "ok"), attribute.String("v", invalid),
		}},
		{"invalid UTF-8 nested in a list", []attribute.KeyValue{
			attribute.Slice("items", attribute.MapValue(attribute.String("k", invalid))),
		}},
		{"excessive depth", []attribute.KeyValue{{Key: "tree", Value: deep}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := NormalizeLogRecordAttributes(tc.attrs)
			require.ErrorIs(t, err, ErrLogEncoding)
			if tc.attrs[0].Key != "" {
				// The empty top-level key is a record rule only; the strict map
				// validator rightly accepts it.
				require.Error(t, ValidateLogAttributes(tc.attrs), "the strict validator must agree")
			}
		})
	}

	_, _, err := NormalizeLogValue(attribute.StringValue(strings.Repeat(invalid, 2)))
	require.ErrorIs(t, err, ErrLogEncoding)

	// An empty key inside a body map has always been valid; only top-level
	// record attribute keys must be nonempty.
	_, _, err = NormalizeLogValue(attribute.MapValue(attribute.KeyValue{Key: "", Value: attribute.IntValue(1)}))
	require.NoError(t, err)
}
