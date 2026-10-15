package controller

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/errkind"
	appLogger "github.com/Laisky/one-api/common/logger"
	dbmodel "github.com/Laisky/one-api/model"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const relayDeliveryPrivate = "SYNTHETIC_PRIVATE_APPLICATION_PAYLOAD"

// relayDeliveryEnvelope captures only the synthetic request sent to the loopback alert API.
type relayDeliveryEnvelope struct {
	Variables struct {
		Type    string `json:"type"`
		Token   string `json:"token"`
		Message string `json:"msg"`
	} `json:"variables"`
}

// relayDeliveryLogger takes t and returns a request logger using the production alert factory and one-per-second limiter plus its delivery channel.
// Cleanup closes the owned transport and joins its worker before the fixture server is discarded.
func relayDeliveryLogger(t *testing.T) (glog.Logger, <-chan relayDeliveryEnvelope) {
	t.Helper()
	deliveries := make(chan relayDeliveryEnvelope, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope relayDeliveryEnvelope
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024)).Decode(&envelope); err != nil {
			t.Errorf("decode synthetic alert envelope: %v", err)
			http.Error(w, "invalid synthetic envelope", http.StatusBadRequest)
			return
		}
		select {
		case deliveries <- envelope:
		default:
			t.Error("unexpected excess synthetic deliveries")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"data":{"TelegramMonitorAlert":{"name":"accepted"}}}`)); err != nil {
			t.Errorf("reply to synthetic alert: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	limiter, err := gutils.NewRateLimiter(ctx, gutils.RateLimiterArgs{Max: 1, NPerSec: 1})
	require.NoError(t, err)
	alert, core, err := appLogger.NewAlertCore(ctx, server.URL, "synthetic-relay-delivery", "synthetic-auth-not-real", limiter)
	require.NoError(t, err)
	t.Cleanup(func() {
		alert.Close()
		cancel()
		select {
		case <-alert.Done():
		case <-time.After(5 * time.Second):
			t.Error("synthetic application alert worker did not stop")
		}
	})
	lg, err := glog.New(glog.WithName("synthetic-one-api.gin"), glog.WithOutputPaths([]string{}),
		glog.WithZapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	require.NoError(t, err)
	return lg.With(zap.String("trace_id", "caller-owned-trace-20261015-01"), zap.Int("user_id", 71002)), deliveries
}

// TestRelayAlertSenderToDelivery takes t and verifies real processor outcomes survive serialization, rate admission, explicit call-site redaction and HTTP delivery.
// All requests use a loopback webhook and synthetic provider errors; no provider or production request is made.
func TestRelayAlertSenderToDelivery(t *testing.T) {
	for _, scenario := range []struct {
		name, mode, outcome string
		duration            time.Duration
	}{
		{"persisted", "provider", "persisted", 37 * time.Second},
		{"write_failed", "write_failed", "failed", 37 * time.Second},
		{"missing_ability", "missing", "failed", 37 * time.Second},
		{"synthesized_500_after_http_200", "body_reset", "not_attempted", 37 * time.Second},
		{"raw_body_explicitly_redacted", "raw_body", "persisted", 37 * time.Second},
		{"ordinary_reason_preserved", "reason", "persisted", 37 * time.Second},
		{"zero_configured_duration", "provider", "persisted", 0},
		{"negative_configured_duration", "provider", "persisted", -time.Second},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newSuspension5XXFixture(t, nil)
			config.ChannelSuspendSecondsFor5XX = scenario.duration
			fixture.params.UserId = 71002
			fixture.params.RequestURL = "/v1/chat/completions?key=" + relayDeliveryPrivate
			fixture.params.Err.StatusCode = http.StatusServiceUnavailable
			switch scenario.mode {
			case "write_failed":
				require.NoError(t, fixture.db.Exec("CREATE TRIGGER reject_delivery_suspension BEFORE UPDATE OF suspend_until ON abilities BEGIN SELECT RAISE(ABORT, 'synthetic update rejected'); END").Error)
			case "missing":
				require.NoError(t, fixture.db.Where("\"group\" = ? AND model = ? AND channel_id = ?", suspension5XXGroup, suspension5XXModel, suspension5XXChannel).Delete(&dbmodel.Ability{}).Error)
			case "body_reset":
				// The actual handler sees HTTP 200 and synthesizes a normalized 500 after a read failure.
				fixture.params.Err = *incidentChannelImpactHandlerError(t, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET})
				require.Equal(t, http.StatusInternalServerError, fixture.params.Err.StatusCode)
			case "raw_body", "reason":
				body := "<html>" + relayDeliveryPrivate + "</html>"
				if scenario.mode == "reason" {
					body = `{"error":{"message":"synthetic provider rejected max_tokens=0","type":"upstream_error","code":"synthetic_parameter"}}`
				}
				response := &http.Response{StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
				fixture.params.Err = *relaycontroller.RelayErrorHandler(response)
				if scenario.mode == "raw_body" {
					require.Contains(t, fixture.params.Err.Message, relayDeliveryPrivate, "the client-facing provider response remains unchanged")
				}
			}
			lg, deliveries := relayDeliveryLogger(t)
			processChannelRelayError(gmw.SetLogger(context.Background(), lg), fixture.params)
			var delivery relayDeliveryEnvelope
			select {
			case delivery = <-deliveries:
			case <-time.After(5 * time.Second):
				t.Fatal("actual application alert was not delivered")
			}
			require.Equal(t, "synthetic-relay-delivery", delivery.Variables.Type)
			require.Equal(t, "synthetic-auth-not-real", delivery.Variables.Token)
			require.NotContains(t, delivery.Variables.Message, relayDeliveryPrivate)
			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(delivery.Variables.Message), &got))
			for key, want := range map[string]any{
				"logger": "synthetic-one-api.gin", "level": "error",
				"trace_id": "caller-owned-trace-20261015-01", "user_id": float64(71002),
				"request_id": fixture.params.RequestID, "channel_id": float64(suspension5XXChannel),
				"channel_name": fixture.params.ChannelName, "group": suspension5XXGroup,
				"origin_model": suspension5XXModel, "actual_model": fixture.params.ActualModel,
				"normalized_status_code": float64(fixture.params.Err.StatusCode),
				"status_semantics":       "normalized_relay_status",
				"error_kind":             relayFailureKind(&fixture.params.Err).String(),
				"suspension_outcome":     scenario.outcome,
				"request_url":            "/v1/chat/completions", "request_url_redacted": true,
			} {
				require.Equal(t, want, got[key], key)
			}
			basis := "normalized_status_fallback"
			if errkind.Of(fixture.params.Err.RawError) != errkind.Unknown {
				basis = "explicit_error_kind"
			}
			require.Equal(t, basis, got["error_kind_basis"])
			require.NotContains(t, got, "upstream_http_status", "the processor does not know an original upstream response status")
			require.NotContains(t, strings.ToLower(delivery.Variables.Message), "upstream server error")
			switch scenario.outcome {
			case "persisted":
				require.Equal(t, "ability suspension persisted after relay failure (normalized 5xx)", got["message"], "the complete outcome must win the production limiter before the generic error")
				require.Equal(t, float64(scenario.duration.Milliseconds()), got["suspension_duration_ms"])
			case "failed":
				require.Equal(t, "ability suspension failed after relay failure (normalized 5xx)", got["message"])
				require.Equal(t, float64(scenario.duration.Milliseconds()), got["suspension_duration_ms"])
			default:
				require.Equal(t, "relay error", got["message"])
				require.NotContains(t, got, "suspension_duration_ms")
			}
			if scenario.mode == "raw_body" {
				require.Equal(t, "[REDACTED]", got["upstream_error"])
				require.Contains(t, fixture.params.Err.Message, relayDeliveryPrivate)
			}
			if scenario.mode == "reason" {
				require.Equal(t, "synthetic provider rejected max_tokens=0", got["upstream_error"])
			}
			if scenario.mode == "body_reset" {
				var rows []dbmodel.Ability
				require.NoError(t, fixture.db.Find(&rows).Error)
				for _, row := range rows {
					require.Nil(t, row.SuspendUntil, "a synthesized status must retain its existing health exclusion")
				}
			}
		})
	}
}

// TestRelayAlertDiagnosticClassificationLabel takes t and confirms explicit server attribution remains distinct from a normalized status fallback without changing policy.
func TestRelayAlertDiagnosticClassificationLabel(t *testing.T) {
	fixture := newSuspension5XXFixture(t, nil)
	fixture.params.UserId = 71002
	fixture.params.Err.RawError = errkind.Mark(errors.New("synthetic explicitly classified failure"), errkind.Server)
	fixture.params.Err.Type = relaymodel.ErrorTypeUpstream
	lg, deliveries := relayDeliveryLogger(t)
	processChannelRelayError(gmw.SetLogger(context.Background(), lg), fixture.params)
	select {
	case delivery := <-deliveries:
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(delivery.Variables.Message), &got))
		require.Equal(t, "server", got["error_kind"])
		require.Equal(t, "explicit_error_kind", got["error_kind_basis"])
	case <-time.After(5 * time.Second):
		t.Fatal("explicit classification alert was not delivered")
	}
}
