package logger

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestUtilityAlertPolicyPreservesSafeMessage exercises the configured application hook.
// It proves real delivery of the ordinary message while excluding synthetic fields.
func TestUtilityAlertPolicyPreservesSafeMessage(t *testing.T) {
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
	var err error
	Logger, err = glog.New(glog.WithName("utility-policy-fixture"), glog.WithLevel(glog.LevelInfo),
		glog.WithOutputPaths([]string{filepath.Join(t.TempDir(), "application.log")}))
	require.NoError(t, err)
	config.LogPushAPI = server.URL
	config.LogPushType = "synthetic-only"
	config.LogPushToken = "synthetic-delivery-only"
	config.DebugEnabled = false
	SetupEnhancedLogger(ctx)
	Logger.Error("safe consumer alert fixture",
		zap.String("token", "synthetic-sensitive-field"),
		zap.Error(errors.New("synthetic-sensitive-error")))
	select {
	case message := <-received:
		require.Contains(t, message, "safe consumer alert fixture", "negative controls must not suppress normal delivery")
		require.NotContains(t, message, "synthetic-sensitive-field")
		require.NotContains(t, message, "synthetic-sensitive-error")
	case <-time.After(3 * time.Second):
		t.Fatal("configured alert did not reach the synthetic endpoint")
	}
	require.NoError(t, Logger.Sync())
}
