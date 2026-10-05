package cohere

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecurityCohereAdmissionReceipt distinguishes authoritative rejection from
// uncertain server failures and ensures a prior rejection never survives reset.
func TestSecurityCohereAdmissionReceipt(t *testing.T) {
	for _, status := range []int{200, 400, 401, 402, 403, 404, 408, 422, 429, 499, 500, 502, 503, 504} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		recordAdmissionRejection(c, &http.Response{StatusCode: status})
		rejected := status == 400 || status == 401 || status == 402 || status == 403 || status == 404 || status == 429
		require.Equal(t, rejected, RejectedBeforeInference(c), "status %d", status)
		ClearRejection(c)
		require.False(t, RejectedBeforeInference(c))
		recordAdmissionRejection(c, nil)
		require.False(t, RejectedBeforeInference(c))
	}
}
