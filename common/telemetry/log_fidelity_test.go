package telemetry

import (
	"compress/gzip"
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger/otelbridge"
)

// fidelityHTTPProvider uses the production queue/batch/exporter against a local
// peer. Every input is synthetic; no external endpoint or inherited env is used.
func fidelityHTTPProvider(t *testing.T, res *sdkresource.Resource, count, length int) (otellog.LoggerProvider, func() []*logspb.ResourceLogs) {
	t.Helper()
	clearLogExporterTestEnvironment(t)
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	received := make(chan []byte, 16)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				w.WriteHeader(400)
				return
			}
			defer gz.Close()
			body = gz
		}
		data, err := io.ReadAll(io.LimitReader(body, 1<<20))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		received <- data
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
	}))
	t.Cleanup(peer.Close)
	endpoint, insecure, cnt, ln := config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure, config.AppLogOTLPMaxAttributes, config.AppLogOTLPMaxAttributeValueBytes
	config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure = strings.TrimPrefix(peer.URL, "http://"), true
	config.AppLogOTLPMaxAttributes, config.AppLogOTLPMaxAttributeValueBytes = count, length
	t.Cleanup(func() {
		config.OpenTelemetryEndpoint, config.OpenTelemetryInsecure = endpoint, insecure
		config.AppLogOTLPMaxAttributes, config.AppLogOTLPMaxAttributeValueBytes = cnt, ln
	})
	p, err := newLoggerProvider(context.Background(), res)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.Shutdown(context.Background())) })
	previousGlobal, previousHolder := global.GetLoggerProvider(), otelbridge.Shared
	otelbridge.Shared = otelbridge.NewProviderHolder()
	require.NoError(t, installLoggerProvider(p))
	t.Cleanup(func() { global.SetLoggerProvider(previousGlobal); otelbridge.Shared = previousHolder })
	collect := func() []*logspb.ResourceLogs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, p.ForceFlush(ctx))
		var groups []*logspb.ResourceLogs
		var payloads [][]byte
		for {
			select {
			case data := <-received:
				payloads = append(payloads, data)
				var request collectorlogs.ExportLogsServiceRequest
				require.NoError(t, proto.Unmarshal(data, &request))
				groups = append(groups, request.ResourceLogs...)
			default:
				for _, group := range groups {
					require.Len(t, group.GetResource().GetAttributes(), len(res.Attributes()), "only supplied synthetic resource fields may reach a wire receipt")
					actual := fidelityPBAttrs(group.GetResource().GetAttributes())
					for _, kv := range res.Attributes() {
						require.Equal(t, kv.Value.AsString(), actual[string(kv.Key)].GetStringValue())
					}
				}
				if dir := os.Getenv("ONEAPI_TEST_FIDELITY_OTLP_RECEIPT_DIR"); dir != "" && len(payloads) == 1 {
					name := map[string]string{"TestLogFidelityProductionMetadataAndBody": "native-metadata", "TestLogFidelityProductionNoImplicitCaps": "zap-content"}[t.Name()]
					if name != "" {
						payload := append([]byte(nil), payloads[0]...)
						t.Cleanup(func() {
							if !t.Failed() {
								writeFidelityWireReceipt(t, dir, name, payload)
							}
						})
					}
				}
				return groups
			}
		}
	}
	return global.GetLoggerProvider(), collect
}

// fidelityPBAttrs exposes protocol scalar/map/list values without JSON rounding.
func fidelityPBAttrs(kvs []*commonpb.KeyValue) map[string]*commonpb.AnyValue {
	out := map[string]*commonpb.AnyValue{}
	for _, kv := range kvs {
		out[kv.Key] = kv.Value
	}
	return out
}

// TestLogFidelityProductionMetadataAndBody preserves protocol namespaces and
// native structured Body/EventName/severity through the actual HTTP exporter.
func TestLogFidelityProductionMetadataAndBody(t *testing.T) {
	resource := sdkresource.NewSchemaless(attribute.String("service.name", "synthetic-oneapi"), attribute.String("custom.resource", "synthetic resource"))
	p, collect := fidelityHTTPProvider(t, resource, 0, 0)
	native := p.Logger("synthetic.vendor", otellog.WithInstrumentationVersion("v-test"), otellog.WithSchemaURL("https://synthetic.invalid/schema"),
		otellog.WithInstrumentationAttributes(attribute.String("scope.label", "scope value")))
	var r otellog.Record
	r.SetBody(attribute.MapValue(attribute.String("nested", "多行\nbody"), attribute.Slice("list", attribute.Int64Value(7), attribute.Value{})))
	r.SetEventName("operation token=SYNTHETIC_APPLICATION_EVENT")
	r.SetSeverity(otellog.SeverityInfo)
	r.SetSeverityText("application-info")
	r.AddAttributes(attribute.Float64("nonfinite", math.NaN()), attribute.String("Authorization", "SYNTHETIC_APPLICATION_VALUE"), attribute.String("service.name", "record-level-name"),
		attribute.ByteSlice("binary", []byte{0, 1, 255}), attribute.KeyValue{Key: "empty", Value: attribute.Value{}})
	native.Emit(context.Background(), r)
	groups := collect()
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Resource.Attributes, 2)
	require.Equal(t, "synthetic-oneapi", fidelityPBAttrs(groups[0].Resource.Attributes)["service.name"].GetStringValue())
	require.Empty(t, groups[0].SchemaUrl)
	require.Equal(t, "synthetic resource", fidelityPBAttrs(groups[0].Resource.Attributes)["custom.resource"].GetStringValue())
	scopes := groups[0].ScopeLogs
	require.Len(t, scopes, 1)
	require.Equal(t, "synthetic.vendor", scopes[0].Scope.Name)
	require.Equal(t, "v-test", scopes[0].Scope.Version)
	require.Equal(t, "https://synthetic.invalid/schema", scopes[0].SchemaUrl)
	require.Len(t, scopes[0].Scope.Attributes, 1)
	require.Equal(t, "scope value", fidelityPBAttrs(scopes[0].Scope.Attributes)["scope.label"].GetStringValue())
	records := scopes[0].LogRecords
	require.Len(t, records, 1)
	got := records[0]
	require.Equal(t, r.EventName(), got.EventName)
	require.Equal(t, "application-info", got.SeverityText)
	require.Equal(t, "多行\nbody", fidelityPBAttrs(got.Body.GetKvlistValue().Values)["nested"].GetStringValue())
	require.Len(t, got.Attributes, 5)
	attrs := fidelityPBAttrs(got.Attributes)
	require.Equal(t, "SYNTHETIC_APPLICATION_VALUE", attrs["Authorization"].GetStringValue())
	require.True(t, math.IsNaN(attrs["nonfinite"].GetDoubleValue()))
	require.Equal(t, "record-level-name", attrs["service.name"].GetStringValue())
	require.Equal(t, []byte{0, 1, 255}, attrs["binary"].GetBytesValue())
	require.Contains(t, attrs, "empty")
}

// TestLogFidelityProductionNoImplicitCaps covers the former hidden128/4096
// defaults while retaining exact application text and ordinary typed attributes.
func TestLogFidelityProductionNoImplicitCaps(t *testing.T) {
	p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
	holder := otelbridge.NewProviderHolder()
	require.NoError(t, holder.Install(p))
	lg := zap.New(otelbridge.NewCore(holder, otelbridge.DefaultScopeName, zapcore.InfoLevel))
	fields := []zap.Field{zap.String("long", strings.Repeat("多行\n", 2000))}
	for i := range 150 {
		fields = append(fields, zap.Int(string(rune(0x400+i)), i))
	}
	original := "read body: SYNTHETIC_APPLICATION_BODY\n原文"
	lg.Info(original, fields...)
	groups := collect()
	require.Len(t, groups, 1)
	records := groups[0].ScopeLogs[0].LogRecords
	require.Len(t, records, 1)
	require.Equal(t, original, records[0].Body.GetStringValue())
	require.Len(t, records[0].Attributes, 151)
	require.Equal(t, strings.Repeat("多行\n", 2000), fidelityPBAttrs(records[0].Attributes)["long"].GetStringValue())
}

// TestLogFidelityExplicitOperatorLimitRejectsWholeRecord distinguishes an
// explicit operator budget from protocol and default content fidelity.
func TestLogFidelityExplicitOperatorLimitRejectsWholeRecord(t *testing.T) {
	spy := installLogExportSpy(t)
	p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 1, 32)
	lg := p.Logger(otelbridge.DefaultScopeName)
	for _, attrs := range [][]attribute.KeyValue{{attribute.String("phase", "ready")}, {attribute.String("phase", "ready"), attribute.Int("count", 3)}} {
		var r otellog.Record
		r.SetBody(attribute.StringValue("worker completed"))
		r.AddAttributes(attrs...)
		lg.Emit(context.Background(), r)
	}
	groups := collect()
	require.Len(t, groups, 1)
	require.Len(t, groups[0].ScopeLogs[0].LogRecords, 1)
	require.Equal(t, 1, spy.outcome("dropped_operator_limit"))
}

// TestLogFidelityInvalidUTF8IsExplicitFailure prevents silent byte repair before
// protobuf transport, while the valid Unicode control above remains accepted.
func TestLogFidelityInvalidUTF8IsExplicitFailure(t *testing.T) {
	spy := installLogExportSpy(t)
	p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
	var r otellog.Record
	r.SetBody(attribute.StringValue("invalid " + string([]byte{255})))
	p.Logger(otelbridge.DefaultScopeName).Emit(context.Background(), r)
	require.Empty(t, collect())
	require.Equal(t, 1, spy.outcome("dropped_encoding"))
}

// TestLogFidelityNativeDuplicatesRejectWholeRecord proves the SDK does not
// silently deduplicate body/attribute maps before validation.
func TestLogFidelityNativeDuplicatesRejectWholeRecord(t *testing.T) {
	spy := installLogExportSpy(t)
	p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
	lg := p.Logger("synthetic.native")
	var attrs otellog.Record
	attrs.SetBody(attribute.StringValue("duplicate attrs"))
	attrs.AddAttributes(attribute.Int("key", 1), attribute.Int("key", 2))
	lg.Emit(context.Background(), attrs)
	var body otellog.Record
	body.SetBody(attribute.MapValue(attribute.Int("key", 1), attribute.Int("key", 2)))
	lg.Emit(context.Background(), body)
	require.Empty(t, collect())
	require.Equal(t, 2, spy.outcome("dropped_encoding"))
}

// writeFidelityWireReceipt writes actual HTTP protobuf bytes only after the test
// has validated its complete synthetic resource/scope and record contract.
func writeFidelityWireReceipt(t *testing.T, dir, name string, payload []byte) {
	t.Helper()
	absolute, err := filepath.Abs(dir)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(absolute, filepath.Clean(os.TempDir())+string(os.PathSeparator)))
	require.NoError(t, os.MkdirAll(absolute, 0700))
	var request collectorlogs.ExportLogsServiceRequest
	require.NoError(t, proto.Unmarshal(payload, &request))
	encoded, err := protojson.MarshalOptions{Indent: "  "}.Marshal(&request)
	require.NoError(t, err)
	for suffix, data := range map[string][]byte{".pb": payload, ".json": encoded} {
		file, err := os.OpenFile(filepath.Join(absolute, name+suffix), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		require.NoError(t, err, "wire receipts are immutable; choose a new versioned directory")
		_, err = file.Write(data)
		require.NoError(t, err)
		require.NoError(t, file.Close())
	}
}

// TestLogFidelityInstalledPublicRejectsInvalidInput covers original records before
// the pinned SDK can silently discard invalid attributes. Values are synthetic.
func TestLogFidelityInstalledPublicRejectsInvalidInput(t *testing.T) {
	spy := installLogExportSpy(t)
	p, collect := fidelityHTTPProvider(t, sdkresource.Empty(), 0, 0)
	lg := p.Logger("synthetic.public")
	for _, attrs := range [][]attribute.KeyValue{
		{attribute.String("", "empty key")},
		{attribute.String("invalid", string([]byte{255}))},
	} {
		var r otellog.Record
		r.SetBody(attribute.StringValue("invalid record"))
		r.AddAttributes(attrs...)
		lg.Emit(context.Background(), r)
	}
	var event otellog.Record
	event.SetEventName(string([]byte{255}))
	event.SetBody(attribute.Value{})
	lg.Emit(context.Background(), event)
	require.Empty(t, collect())
	require.Equal(t, 3, spy.outcome("dropped_encoding"))
}
