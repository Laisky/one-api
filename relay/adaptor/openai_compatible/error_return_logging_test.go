package openai_compatible

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
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestMalformedResponseReturnsWithoutLogging ensures terminal handling owns returned parse failures.
func TestMalformedResponseReturnsWithoutLogging(t *testing.T) {
	tests := []struct {
		name, code string
		call       func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage)
	}{
		{"embedding", "unmarshal_embedding_response_failed", EmbeddingHandler},
		{"chat", "unmarshal_response_body_failed", func(c *gin.Context, r *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
			return Handler(c, r, 1, "test")
		}},
		{"thinking", "unmarshal_response_body_failed", func(c *gin.Context, r *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
			return HandlerWithThinking(c, r, 1, "test")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewConsoleWithName("test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			gmw.SetLogger(c, lg)
			failure, usage := tc.call(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{"))})
			require.NotNil(t, failure)
			require.Equal(t, tc.code, failure.Code)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			require.Equal(t, "unexpected end of JSON input", failure.Message)
			require.NotNil(t, failure.RawError)
			var syntaxErr *json.SyntaxError
			require.True(t, stderrors.As(failure.RawError, &syntaxErr))
			var trace interface{ StackTrace() stackerrors.StackTrace }
			require.True(t, stderrors.As(failure.RawError, &trace))
			require.Contains(t, fmt.Sprintf("%+v", trace.StackTrace()), "Handler")
			require.Nil(t, usage)
			require.Empty(t, rec.Body.String())
			require.Empty(t, observed.FilterLevelExact(zapcore.ErrorLevel).All(), "returned error is logged at the terminal boundary")
		})
	}
}
