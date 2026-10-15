package logger

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestUtilityAlertPolicyPreservesSafeMessage takes t, verifies caller-controlled fields and explicit redaction through SetupEnhancedLogger and both sinks, and returns no value.
func TestUtilityAlertPolicyPreservesSafeMessage(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		redact bool
	}{
		{"unmarked_caller_fields_preserved", false},
		{"explicit_caller_redaction", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			restoreLogPushConfig(t)
			original := Logger
			originalDebug := config.DebugEnabled
			t.Cleanup(func() { Logger = original; config.DebugEnabled = originalDebug })
			received := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
				if err != nil {
					t.Errorf("read synthetic alert: %v", err)
				}
				var payload struct{ Variables map[string]string }
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Errorf("decode synthetic alert: %v", err)
				}
				received <- payload.Variables["msg"]
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, `{"data":{"TelegramMonitorAlert":{"Name":"fixture"}}}`); err != nil {
					t.Errorf("write synthetic alert: %v", err)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			localPath := filepath.Join(t.TempDir(), "application.log")
			var err error
			Logger, err = glog.New(glog.WithName("utility-policy-fixture"), glog.WithLevel(glog.LevelInfo), glog.WithOutputPaths([]string{localPath}))
			require.NoError(t, err)
			config.LogPushAPI = server.URL
			config.LogPushType = "synthetic-only"
			config.LogPushToken = "synthetic-delivery-only"
			config.DebugEnabled = false
			SetupEnhancedLogger(ctx)
			const fieldValue = "synthetic-sensitive-field"
			const errorValue = "synthetic-sensitive-error"
			callerError := errors.New(errorValue)
			fields := []zap.Field{zap.String("token", fieldValue), zap.Error(callerError)}
			if scenario.redact {
				fields = []zap.Field{zap.String("token", Redact(fieldValue)), zap.String("error", Redact(callerError))}
			}
			Logger.With(zap.String("ordinary_context", "fixture-context")).Error("safe consumer alert fixture", fields...)
			var message string
			select {
			case message = <-received:
			case <-time.After(3 * time.Second):
				t.Fatal("configured alert did not reach the synthetic endpoint")
			}
			var delivered map[string]any
			require.NoError(t, json.Unmarshal([]byte(message), &delivered))
			require.Equal(t, "safe consumer alert fixture", delivered["message"])
			require.Equal(t, "fixture-context", delivered["ordinary_context"])
			require.NoError(t, Logger.Sync())
			local, err := os.ReadFile(localPath)
			require.NoError(t, err)
			require.Contains(t, string(local), "safe consumer alert fixture")
			require.Contains(t, string(local), "fixture-context")
			if scenario.redact {
				require.Equal(t, "[REDACTED]", delivered["token"])
				require.Equal(t, "[REDACTED]", delivered["error"])
				for _, rendered := range []string{message, string(local)} {
					require.NotContains(t, rendered, fieldValue)
					require.NotContains(t, rendered, errorValue)
					require.Contains(t, rendered, "[REDACTED]")
				}
			} else {
				require.Equal(t, fieldValue, delivered["token"])
				require.Equal(t, errorValue, delivered["error"])
				for _, rendered := range []string{message, string(local)} {
					require.Contains(t, rendered, fieldValue)
					require.Contains(t, rendered, errorValue)
				}
			}
		})
	}
}
