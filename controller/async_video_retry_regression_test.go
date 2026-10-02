package controller

import (
 "errors"
 "net/http"
 "net/http/httptest"
 "testing"

 "github.com/gin-gonic/gin"
 "github.com/Laisky/one-api/common/ctxkey"
 "github.com/Laisky/one-api/relay/adaptor/openai"
)

// TestAsyncVideoSubmissionRetryBoundary exercises the real retry decision, not
// source text: a failed acknowledgement must never create another paid job.
func TestAsyncVideoSubmissionRetryBoundary(t *testing.T) {
 for _, path := range []string{"/v1/videos", "/v1/videos/generations", "/v1/async/videos"} {
  t.Run(path, func(t *testing.T) {
   c, _ := gin.CreateTestContext(httptest.NewRecorder())
   c.Request = httptest.NewRequest(http.MethodPost, path, nil)
   failure := openai.ErrorWrapper(errors.New("upstream acknowledgement lost"), "do_request_failed", http.StatusBadGateway)
   if err := shouldRetry(c, failure); err != nil {
    t.Fatalf("unforwarded request should remain retryable: %v", err)
   }
   c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
   if err := shouldRetry(c, failure); err == nil {
    t.Fatal("possibly accepted paid submission must not be replayed")
   }
  })
 }
}
