package render

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/zap/zapcore"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	commonsse "github.com/songquanpeng/one-api/common/sse"
)

// TestClassifyHeartbeatLineReaderLogPlan verifies HeartbeatLineReader errors are classified correctly.
func TestClassifyHeartbeatLineReaderLogPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		wantLevel       zapcore.Level
		wantMessage     string
		wantTermination string
	}{
		{
			name:            "client canceled",
			err:             context.Canceled,
			wantLevel:       zapcore.DebugLevel,
			wantMessage:     "stream stopped after downstream cancellation",
			wantTermination: "client_canceled",
		},
		{
			name:            "client deadline exceeded",
			err:             context.DeadlineExceeded,
			wantLevel:       zapcore.DebugLevel,
			wantMessage:     "stream stopped after downstream deadline",
			wantTermination: "client_deadline_exceeded",
		},
		{
			name:            "line too long",
			err:             commonsse.ErrLineTooLong,
			wantLevel:       zapcore.ErrorLevel,
			wantMessage:     "stream line exceeded reader limit",
			wantTermination: "malformed_upstream_line",
		},
		{
			name:            "unexpected error",
			err:             errors.New("boom"),
			wantLevel:       zapcore.ErrorLevel,
			wantMessage:     "error reading stream",
			wantTermination: "unexpected_error",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := classifyHeartbeatLineReaderLogPlan(tt.err)
			require.Equal(t, tt.wantLevel, plan.level)
			require.Equal(t, tt.wantMessage, plan.message)
			require.Equal(t, tt.wantTermination, plan.termination)
		})
	}
}

// TestBuildHeartbeatLineReaderLogFields verifies HeartbeatLineReader fields keep heartbeat diagnostics.
func TestBuildHeartbeatLineReaderLogFields(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	hbr := &HeartbeatLineReader{
		heartbeatsSent:    2,
		heartbeatWriteErr: errors.New("broken pipe"),
	}
	plan := classifyHeartbeatLineReaderLogPlan(commonsse.ErrLineTooLong)
	fields := buildHeartbeatLineReaderLogFields(c, commonsse.ErrLineTooLong, hbr, plan)

	encoder := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(encoder)
	}

	require.Equal(t, "malformed_upstream_line", encoder.Fields["stream_termination"])
	require.EqualValues(t, 2, encoder.Fields["heartbeats_sent"])
	require.Equal(t, "context canceled", encoder.Fields["client_context_state"])
	require.Contains(t, fmt.Sprint(encoder.Fields["heartbeat_write_error"]), "broken pipe")
	_, exists := encoder.Fields["scanner_max_token_size"]
	require.False(t, exists)
}
