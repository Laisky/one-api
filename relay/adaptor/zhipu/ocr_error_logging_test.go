package zhipu

import (
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

type trackedOCRFaultBody struct {
	io.Reader
	fault  error
	closed bool
}

// Read reports the transport fault after delivering the complete or partial body.
func (b *trackedOCRFaultBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		return n, b.fault
	}
	return n, err
}

// Close records cleanup of the upstream body.
func (b *trackedOCRFaultBody) Close() error { b.closed = true; return nil }

// TestOCRReadFailureIsReturnedOnce verifies receipt evidence, cleanup and diagnostic context without logging a returned error.
func TestOCRReadFailureIsReturnedOnce(t *testing.T) {
	const complete = `{"usage":{"prompt_tokens":8,"completion_tokens":4,"total_tokens":12},"data_info":{"num_pages":1}}`
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{{"complete", complete, true}, {"partial", complete[:len(complete)-6], false}} {
		t.Run(tc.name, func(t *testing.T) {
			base, err := glog.NewConsoleWithName("ocr", glog.LevelDebug)
			require.NoError(t, err)
			core, logs := observer.New(zapcore.DebugLevel)
			logger := base.WithOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			gmw.SetLogger(c, logger)
			fault := stderrors.New("private transport error")
			body := &trackedOCRFaultBody{Reader: strings.NewReader(tc.body), fault: fault}
			receipt, apiErr := forwardOCRResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body})
			require.True(t, body.closed)
			require.NotNil(t, apiErr)
			require.ErrorIs(t, apiErr.RawError, fault)
			require.Equal(t, "read_response_body_failed", apiErr.Code)
			require.Empty(t, recorder.Body.String())
			if tc.valid {
				require.Equal(t, 12, receipt.Usage.TotalTokens)
				require.Equal(t, 1, receipt.Pages)
			} else {
				require.Nil(t, receipt.Usage)
				require.Equal(t, model.OCRReceiptUnreadable, receipt.UsageProblem)
			}
			entries := logs.FilterMessage("upstream OCR response read failed").All()
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			require.EqualValues(t, len(tc.body), fields["body_bytes"])
			require.Equal(t, receipt.UsageProblem, fields["usage_problem"])
			require.NotContains(t, fields, "error")
		})
	}
}
