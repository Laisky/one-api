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
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	apimetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	recorder "github.com/Laisky/one-api/monitor/otel"
)

// TestPrivateUUIDMetricsPreserveDistinctOperationalSeries uses the real recorder,
// production SDK view and final exporter, including all inactive zero states.
func TestPrivateUUIDMetricsPreserveDistinctOperationalSeries(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader), sdkmetric.WithView(newPrivateMetricView()))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		require.NoError(t, provider.Shutdown(ctx))
	})
	r, err := recorder.NewOtelRecorder()
	require.NoError(t, err)
	active := map[string]string{"primary": "ready", "log": "degraded"}
	for role, current := range active {
		for _, state := range privateCompactStates {
			r.UpdateCompactUUIDState(role, state, current == state)
		}
	}
	wantedBacklog := map[string]float64{}
	for i, target := range []string{"users.uuid", "tokens.user_uuid", "logs.channel_uuid"} {
		role := "primary"
		if strings.HasPrefix(target, "logs.") {
			role = "log"
		}
		for j, kind := range []string{"gap", "blocker", "mismatch"} {
			value := float64(10*i + j + 1)
			r.UpdateCompactUUIDBacklog(role, target, kind, value)
			wantedBacklog[role+"/"+target+"/"+kind] = value
		}
	}
	r.RecordCompactUUIDAction("primary", "cycle", "success")
	r.RecordCompactUUIDAction("log", "audit", "failure")
	r.RecordUUIDBackfillRows("primary", "owned", "users.uuid", "updated", 3)
	r.RecordUUIDBackfillRows("log", "token_name", "logs.token_uuid", "unresolved", 5)
	r.RecordUUIDBackfillCycle("log", "catchup", "failure", time.Second)
	r.RecordUUIDBackfillFinalizer("primary", "failure")
	r.RecordCompactUUIDDuration("primary", "lock", time.Second)
	r.RecordCompactUUIDLookupFallback("log", "expired_health")
	r.UpdateUUIDBackfillBacklog("primary", "all", 1)
	r.UpdateUUIDBackfillBacklog("log", "all", 0)
	r.UpdateCompactUUIDLastProgress("primary", 100)
	r.UpdateCompactUUIDLastProgress("log", 200)
	// Known instrument names must not permit arbitrary values, or a vocabulary
	// belonging to another instrument, via the direct SDK API.
	probe, err := provider.Meter("one-api").Int64Counter("oneapi_compact_uuid_actions_total", apimetric.WithDescription("Total compact UUID DDL, fill, validation, marker, audit, and repair outcomes"))
	require.NoError(t, err)
	probe.Add(ctx, 1, apimetric.WithAttributes(
		attribute.String("role", "sentinel-role"), attribute.String("action", "sentinel-action"),
		attribute.String("result", "sentinel-result"), attribute.String("state", "ready"),
		attribute.String("target", "users.uuid"), attribute.String("cookie", "sentinel-cookie")))
	var original metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &original))
	capture := &metricPrivacyCapture{}
	require.NoError(t, (privacyMetricExporter{Exporter: capture}).Export(ctx, &original))
	metrics := map[string]metricdata.Metrics{}
	for _, scope := range capture.result.ScopeMetrics {
		require.Equal(t, "one-api", scope.Scope.Name)
		for _, metric := range scope.Metrics {
			if metric.Name == "oneapi_metric_privacy_dropped_instruments_total" {
				continue
			}
			_, duplicate := metrics[metric.Name]
			require.False(t, duplicate, metric.Name)
			metrics[metric.Name] = metric
		}
	}
	states := metrics["oneapi_compact_uuid_state"].Data.(metricdata.Gauge[int64]).DataPoints
	require.Len(t, states, 2*len(privateCompactStates))
	seenStates := map[string]int64{}
	activeCounts := map[string]int{}
	for _, point := range states {
		role := uuidMetricLabel(t, point.Attributes, "role")
		state := uuidMetricLabel(t, point.Attributes, "state")
		seenStates[role+"/"+state] = point.Value
		if point.Value == 1 {
			activeCounts[role]++
			require.Equal(t, active[role], state)
		} else {
			require.Zero(t, point.Value)
		}
	}
	require.Len(t, seenStates, len(states), "each zero state must remain distinct")
	require.Equal(t, map[string]int{"primary": 1, "log": 1}, activeCounts)
	backlog := metrics["oneapi_compact_uuid_backlog_rows"].Data.(metricdata.Gauge[float64]).DataPoints
	actualBacklog := map[string]float64{}
	for _, point := range backlog {
		key := uuidMetricLabel(t, point.Attributes, "role") + "/" + uuidMetricLabel(t, point.Attributes, "target") + "/" + uuidMetricLabel(t, point.Attributes, "kind")
		actualBacklog[key] = point.Value
	}
	require.Equal(t, wantedBacklog, actualBacklog)
	actions := metrics["oneapi_compact_uuid_actions_total"].Data.(metricdata.Sum[int64]).DataPoints
	require.Len(t, actions, 3)
	results := map[string]int64{}
	for _, point := range actions {
		for _, kv := range point.Attributes.ToSlice() {
			require.NotContains(t, kv.Value.AsString(), "sentinel")
			require.NotContains(t, []string{"state", "target", "cookie"}, string(kv.Key))
		}
		if point.Attributes.Len() == 0 {
			require.Equal(t, int64(1), point.Value)
			continue
		}
		results[uuidMetricLabel(t, point.Attributes, "result")] = point.Value
	}
	require.Equal(t, map[string]int64{"success": 1, "failure": 1}, results)
	rows := metrics["oneapi_uuid_backfill_rows_total"].Data.(metricdata.Sum[int64]).DataPoints
	rowResults := map[string]int64{}
	for _, point := range rows {
		require.Equal(t, 4, point.Attributes.Len())
		rowResults[uuidMetricLabel(t, point.Attributes, "result")] = point.Value
	}
	require.Equal(t, map[string]int64{"updated": 3, "unresolved": 5}, rowResults)
	for name, enums := range privateUUIDInstrumentEnums {
		for key, values := range enums {
			for _, value := range values {
				require.True(t, privateMetricFilter(name)(attribute.String(key, value)), name+"/"+key+"/"+value)
			}
			require.False(t, privateMetricFilter(name)(attribute.String(key, "sentinel-secret")))
			require.False(t, privateMetricFilter(name)(attribute.Int(key, 7)))
		}
	}
	require.False(t, privateMetricFilter("one_api_relay_requests_total")(attribute.String("role", "primary")))
}

// uuidMetricLabel retrieves a required, privacy-filtered string label.
func uuidMetricLabel(t *testing.T, attrs attribute.Set, key string) string {
	t.Helper()
	value, ok := attrs.Value(attribute.Key(key))
	require.True(t, ok, key)
	require.Equal(t, attribute.STRING, value.Type())
	return value.AsString()
}

// TestPrivateUUIDExporterRejectsUnfilteredValues covers the export boundary
// independently of the SDK view and checks that source records remain untouched.
func TestPrivateUUIDExporterRejectsUnfilteredValues(t *testing.T) {
	for name, enums := range privateUUIDInstrumentEnums {
		t.Run(name, func(t *testing.T) {
			valid := []attribute.KeyValue{}
			invalid := []attribute.KeyValue{attribute.String("success", "true"), attribute.String("cookie", "sentinel-cookie")}
			for key, values := range enums {
				valid = append(valid, attribute.String(key, values[0]))
				invalid = append(invalid, attribute.String(key, "sentinel-secret"))
			}
			original := metricdata.ResourceMetrics{ScopeMetrics: []metricdata.ScopeMetrics{{
				Metrics: []metricdata.Metrics{{Name: name, Data: metricdata.Sum[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{Attributes: attribute.NewSet(valid...), Value: 3},
						{Attributes: attribute.NewSet(invalid...), Value: 5},
					},
				}}},
			}}}
			capture := &metricPrivacyCapture{}
			require.NoError(t, (privacyMetricExporter{Exporter: capture}).Export(context.Background(), &original))
			points := capture.result.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints
			require.Len(t, points, 2)
			require.ElementsMatch(t, valid, points[0].Attributes.ToSlice())
			require.Zero(t, points[1].Attributes.Len())
			require.Equal(t, int64(3), points[0].Value)
			require.Equal(t, int64(5), points[1].Value)
			source := original.ScopeMetrics[0].Metrics[0].Data.(metricdata.Sum[int64]).DataPoints
			require.ElementsMatch(t, invalid, source[1].Attributes.ToSlice())
		})
	}
}

// TestPrivateUUIDCatalogMatchesSource prevents model additions from silently
// collapsing series again. Only source constants and registry literals are read.
func TestPrivateUUIDCatalogMatchesSource(t *testing.T) {
	for _, spec := range []struct {
		file, prefix string
		want         []string
	}{
		{"compact_uuid_state.go", "compactState", privateCompactStates},
		{"uuid_migration_topology.go", "uuidRole", privateUUIDRoles},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", "model", spec.file), nil, 0)
		require.NoError(t, err)
		var values []string
		ast.Inspect(file, func(node ast.Node) bool {
			decl, ok := node.(*ast.ValueSpec)
			if !ok || len(decl.Names) != 1 || len(decl.Values) != 1 || !strings.HasPrefix(decl.Names[0].Name, spec.prefix) {
				return true
			}
			literal, ok := decl.Values[0].(*ast.BasicLit)
			if ok && literal.Kind == token.STRING {
				value, err := strconv.Unquote(literal.Value)
				require.NoError(t, err)
				values = append(values, value)
			}
			return true
		})
		require.ElementsMatch(t, spec.want, values, spec.file)
	}
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", "model", "uuid_migration_registry.go"), nil, 0)
	require.NoError(t, err)
	var targets []string
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		table, column := "", "uuid"
		for _, element := range literal.Elts {
			kv, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || (key.Name != "table" && key.Name != "uuidColumn") {
				continue
			}
			value, ok := kv.Value.(*ast.BasicLit)
			require.True(t, ok)
			decoded, err := strconv.Unquote(value.Value)
			require.NoError(t, err)
			if key.Name == "table" {
				table = decoded
			} else {
				column = decoded
			}
		}
		if table != "" {
			targets = append(targets, table+"."+column)
		}
		return true
	})
	require.ElementsMatch(t, privateUUIDTargets, targets)
}
