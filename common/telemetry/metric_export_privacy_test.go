package telemetry

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	apimetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

// metricPrivacyCapture retains an export for immediate assertions.
type metricPrivacyCapture struct {
	sdkmetric.Exporter
	result *metricdata.ResourceMetrics
}

// Export retains the private view supplied by the export boundary.
func (e *metricPrivacyCapture) Export(_ context.Context, data *metricdata.ResourceMetrics) error {
	e.result = data
	return nil
}

// TestDirectSDKMetricMetadataIsPrivate exercises the global Meter API and proves
// unknown names and scope, description, unit, resource strings cannot escape.
func TestDirectSDKMetricMetadataIsPrivate(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	resource := sdkresource.NewSchemaless(attribute.String("service.name", "one-api"),
		attribute.String("secret", "sentinel-resource-secret"))
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithResource(resource),
		sdkmetric.WithView(newPrivateMetricView()))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previous) })
	meter := otel.Meter("sentinel-scope-secret", apimetric.WithInstrumentationVersion("sentinel-version"),
		apimetric.WithSchemaURL("https://sentinel-schema-secret"),
		apimetric.WithInstrumentationAttributes(attribute.String("secret", "sentinel-scope-attribute")))
	known, err := meter.Int64Counter("one_api_relay_requests_total",
		apimetric.WithDescription("sentinel-description-secret"), apimetric.WithUnit("sentinel-unit"))
	require.NoError(t, err)
	unknown, err := meter.Int64Counter("one_api_sentinel_instrument_secret")
	require.NoError(t, err)
	known.Add(context.Background(), 3, apimetric.WithAttributes(attribute.String("channel_id", "7"), attribute.String("cookie", "sentinel-cookie")))
	unknown.Add(context.Background(), 1)
	var original metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &original))
	capture := &metricPrivacyCapture{}
	require.NoError(t, (privacyMetricExporter{Exporter: capture}).Export(context.Background(), &original))
	require.Equal(t, []attribute.KeyValue{attribute.String("service.name", "one-api")}, capture.result.Resource.Attributes())
	var found, counted bool
	for _, scope := range capture.result.ScopeMetrics {
		require.Equal(t, "one-api", scope.Scope.Name)
		require.Empty(t, scope.Scope.Version)
		require.Empty(t, scope.Scope.SchemaURL)
		require.Equal(t, 0, scope.Scope.Attributes.Len())
		for _, metric := range scope.Metrics {
			require.Empty(t, metric.Description)
			require.Empty(t, metric.Unit)
			require.NotContains(t, metric.Name, "sentinel")
			if metric.Name == "one_api_relay_requests_total" {
				found = true
				points := metric.Data.(metricdata.Sum[int64]).DataPoints
				require.Len(t, points, 1)
				require.Equal(t, int64(3), points[0].Value)
				require.Equal(t, []attribute.KeyValue{attribute.String("channel_id", "7")}, points[0].Attributes.ToSlice())
			} else if metric.Name == "oneapi_metric_privacy_dropped_instruments_total" {
				counted = true
				require.GreaterOrEqual(t, metric.Data.(metricdata.Sum[int64]).DataPoints[0].Value, int64(1))
			}
		}
	}
	require.True(t, found, "known business counts must survive")
	require.True(t, counted, "privacy drops must be observable without private names")
}

// TestMetricRegistryCoversSourceDeclaredInstruments prevents future recorder
// additions from silently losing operational telemetry at the privacy boundary.
func TestMetricRegistryCoversSourceDeclaredInstruments(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "monitor", "otel", "*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	constructors := map[string]bool{"Int64Counter": true, "Float64Counter": true,
		"Int64UpDownCounter": true, "Float64UpDownCounter": true,
		"Int64Gauge": true, "Float64Gauge": true, "Int64Histogram": true, "Float64Histogram": true,
		"Int64ObservableCounter": true, "Float64ObservableCounter": true,
		"Int64ObservableGauge": true, "Float64ObservableGauge": true}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !constructors[selector.Sel.Name] {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			require.True(t, ok, "%s: metric names must be reviewed static literals", path)
			if ok {
				name, err := strconv.Unquote(literal.Value)
				require.NoError(t, err)
				require.True(t, privateMetricInstruments[name], "%s: missing metric %s", path, name)
			}
			return true
		})
	}
}
