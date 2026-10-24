package openai_compatible

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// conversionJoinedEOFBody emits synthetic stream bytes and one unchanged terminal error.
type conversionJoinedEOFBody struct {
	prefix   *strings.Reader
	terminal error
	returned bool
	closes   atomic.Int32
}

// Read returns the selected mixed or pure EOF once, then ordinary EOF.
func (b *conversionJoinedEOFBody) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	if !b.returned {
		b.returned = true
		return 0, b.terminal
	}
	return 0, io.EOF
}

// Close records the converter's ordinary response cleanup without changing read outcomes.
func (b *conversionJoinedEOFBody) Close() error { b.closes.Add(1); return nil }

// TestClaudeConversionJoinedEOFPreservesDiagnostics exercises the actual converter and delivered Claude events.
func TestClaudeConversionJoinedEOFPreservesDiagnostics(t *testing.T) {
	independent := errors.New("synthetic independent Claude conversion transport failure")
	cases := []struct {
		name     string
		terminal error
		cause    error
	}{
		{"joined_unexpected_eof", errors.Join(io.EOF, io.ErrUnexpectedEOF), io.ErrUnexpectedEOF},
		{"joined_independent_error", errors.Join(io.EOF, independent), independent},
		{"unexpected_eof_control", io.ErrUnexpectedEOF, io.ErrUnexpectedEOF},
		{"wrapped_eof_control", errors.Wrap(io.EOF, "synthetic EOF wrapper"), nil},
		{"duplicate_eof_control", errors.Join(io.EOF, io.EOF), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newBridgeTestCtx(t)
			core, observed := observer.New(zapcore.DebugLevel)
			logger, loggerErr := glog.NewConsoleWithName("joined-eof-conversion", glog.LevelDebug,
				zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, loggerErr)
			gmw.SetLogger(c, logger)
			ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			// Observe actual delivered content before a complete measured receipt.
			// No DONE is supplied: only pure EOF may retain the existing no-DONE policy.
			wire := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n" +
				"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7,\"total_tokens\":18}}\n\n"
			body := &conversionJoinedEOFBody{prefix: strings.NewReader(wire), terminal: tc.terminal}
			response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
			usage, failure := ConvertOpenAIStreamToClaudeSSE(c, response, 11, "gpt-4")
			require.NoError(t, ctx.Err(), "the finite fixture must finish without its deadline rescuing it")
			require.True(t, body.returned, "the converter must actually receive the selected terminal error")
			require.Equal(t, int32(1), body.closes.Load())
			require.NotNil(t, usage, "read errors must retain already observed usage")
			require.Equal(t, 11, usage.PromptTokens)
			require.Equal(t, 7, usage.CompletionTokens)
			require.Equal(t, 18, usage.TotalTokens)
			require.Contains(t, rec.Body.String(), "hello", "fixture content must be delivered before the terminal error")
			require.NotContains(t, rec.Body.String(), "event: message_stop", "an interrupted/no-DONE stream cannot fabricate success")
			// Preserve the converter's existing log-and-withhold-terminal contract.
			// Returning a new API error here could change the caller's retry behavior.
			require.Nil(t, failure)
			diagnostics := observed.FilterMessage("error reading stream").All()
			if tc.cause != nil {
				require.Len(t, diagnostics, 1, "an independent read failure must remain visible in the converter diagnostic")
				require.Equal(t, zapcore.ErrorLevel, diagnostics[0].Level)
				fields := diagnostics[0].ContextMap()
				require.Equal(t, "unexpected_error", fields["stream_termination"])
				require.Contains(t, fields["stream_error"], tc.cause.Error())
			} else {
				require.Empty(t, diagnostics, "pure wrapped/duplicate EOF must not acquire a transport-failure diagnostic")
			}
		})
	}
}
