package openai

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	stackerrors "github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestEmbeddingParseFailureReturnsWithoutLogging preserves its response and defers error logging to its caller.
func TestEmbeddingParseFailureReturnsWithoutLogging(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	gmw.SetLogger(c, lg)
	failure, usage := EmbeddingHandler(c, &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{"))}, 1, "test")
	require.NotNil(t, failure)
	require.Equal(t, "unmarshal_embedding_response_failed", failure.Code)
	require.Equal(t, 500, failure.StatusCode)
	require.Equal(t, "unexpected end of JSON input", failure.Message)
	require.NotNil(t, failure.RawError)
	var syntaxErr *json.SyntaxError
	require.True(t, stderrors.As(failure.RawError, &syntaxErr))
	var trace interface{ StackTrace() stackerrors.StackTrace }
	require.True(t, stderrors.As(failure.RawError, &trace))
	require.Contains(t, fmt.Sprintf("%+v", trace.StackTrace()), "EmbeddingHandler")
	require.Nil(t, usage)
	require.Empty(t, rec.Body.String())
	require.Empty(t, observed.FilterLevelExact(zapcore.ErrorLevel).All(), "returned error is logged at the terminal boundary")
}

// TestStreamReadFailureReturnsWithoutLogging verifies transport failure and emitted events survive without duplicate error records.
func TestStreamReadFailureReturnsWithoutLogging(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode int
		call func(*gin.Context, *http.Response, int) (*model.ErrorWithStatusCode, string, *model.Usage)
	}{
		{"direct", relaymode.ResponseAPI, ResponseAPIDirectStreamHandler}, {"converted", relaymode.ChatCompletions, ResponseAPIStreamHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewConsoleWithName("test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			c, rec := newDirectStreamContext()
			gmw.SetLogger(c, lg)
			event := securityReceiptEvent(t, 4)
			body := &securityStreamBody{Reader: strings.NewReader(event), readErr: io.ErrUnexpectedEOF}
			failure, _, usage := tc.call(c, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, tc.mode)
			require.NotNil(t, failure)
			require.Equal(t, "read_stream_failed", failure.Code)
			require.ErrorIs(t, failure.RawError, io.ErrUnexpectedEOF)
			require.NotNil(t, usage)
			require.Equal(t, 18, usage.TotalTokens)
			require.Contains(t, rec.Body.String(), "data:")
			require.NotContains(t, rec.Body.String(), "[DONE]")
			require.EqualValues(t, 1, body.closed.Load())
			require.Empty(t, observed.FilterMessage("stream read failed").All(), "returned error is logged at the terminal boundary")
			progress := observed.FilterMessage("stream forwarding interrupted").All()
			require.Len(t, progress, 1)
			require.EqualValues(t, 1, progress[0].ContextMap()["forwarded_chunks"])
			require.NotContains(t, progress[0].ContextMap(), "error")
		})
	}
}
