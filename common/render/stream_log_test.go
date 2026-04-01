package render

import (
	"bufio"
	"context"
	stdErrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestClassifyHeartbeatScannerLogPlan verifies HeartbeatScanner errors are classified
// into the expected log message, level, and scanner-limit behavior.
// Parameters:
//   - t: the test context.
//
// Returns:
//   - nothing.
func TestClassifyHeartbeatScannerLogPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		err                error
		wantLevel          zapcore.Level
		wantMessage        string
		wantTermination    string
		wantIncludeScanner bool
	}{
		{
			name:               "client canceled",
			err:                context.Canceled,
			wantLevel:          zapcore.DebugLevel,
			wantMessage:        "stream stopped after downstream cancellation",
			wantTermination:    "client_canceled",
			wantIncludeScanner: false,
		},
		{
			name:               "client deadline exceeded",
			err:                context.DeadlineExceeded,
			wantLevel:          zapcore.DebugLevel,
			wantMessage:        "stream stopped after downstream deadline",
			wantTermination:    "client_deadline_exceeded",
			wantIncludeScanner: false,
		},
		{
			name:               "scanner token too long",
			err:                bufio.ErrTooLong,
			wantLevel:          zapcore.ErrorLevel,
			wantMessage:        "stream token exceeded scanner limit",
			wantTermination:    "scanner_token_too_long",
			wantIncludeScanner: true,
		},
		{
			name:               "unexpected scanner error",
			err:                stdErrors.New("boom"),
			wantLevel:          zapcore.ErrorLevel,
			wantMessage:        "error reading stream",
			wantTermination:    "unexpected_error",
			wantIncludeScanner: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := classifyHeartbeatScannerLogPlan(tt.err)
			require.Equal(t, tt.wantLevel, plan.level)
			require.Equal(t, tt.wantMessage, plan.message)
			require.Equal(t, tt.wantTermination, plan.termination)
			require.Equal(t, tt.wantIncludeScanner, plan.includeScannerMaxTokenSize)
		})
	}
}

// TestBuildHeartbeatScannerLogFields_ClientCancellation verifies client cancellation fields
// carry heartbeat diagnostics while omitting the scanner limit for expected disconnects.
// Parameters:
//   - t: the test context.
//
// Returns:
//   - nothing.
func TestBuildHeartbeatScannerLogFields_ClientCancellation(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	hbs := &HeartbeatScanner{
		heartbeatsSent:    3,
		heartbeatWriteErr: stdErrors.New("broken pipe"),
	}
	plan := classifyHeartbeatScannerLogPlan(context.Canceled)
	fields := buildHeartbeatScannerLogFields(c, context.Canceled, hbs, 1024, plan)

	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}

	require.Equal(t, "client_canceled", encoder.Fields["stream_termination"])
	require.Equal(t, "context canceled", encoder.Fields["client_context_state"])
	require.EqualValues(t, 3, encoder.Fields["heartbeats_sent"])
	require.Contains(t, fmt.Sprint(encoder.Fields["heartbeat_write_error"]), "broken pipe")
	_, exists := encoder.Fields["scanner_max_token_size"]
	require.False(t, exists)
}

// TestBuildHeartbeatScannerLogFields_ScannerFailure verifies real scanner failures include
// the configured scanner limit so token-size problems remain diagnosable.
// Parameters:
//   - t: the test context.
//
// Returns:
//   - nothing.
func TestBuildHeartbeatScannerLogFields_ScannerFailure(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	plan := classifyHeartbeatScannerLogPlan(bufio.ErrTooLong)
	fields := buildHeartbeatScannerLogFields(c, bufio.ErrTooLong, nil, 2048, plan)

	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}

	require.Equal(t, "scanner_token_too_long", encoder.Fields["stream_termination"])
	require.EqualValues(t, 2048, encoder.Fields["scanner_max_token_size"])
	_, exists := encoder.Fields["client_context_state"]
	require.False(t, exists)
}
