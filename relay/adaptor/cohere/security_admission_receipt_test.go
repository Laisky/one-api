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
	for _, status := range []int{200, 400, 401, 403, 404, 429, 499, 500, 502, 503, 504} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		recordAdmissionRejection(c, &http.Response{StatusCode: status})
		require.Equal(t, status == 401 || status == 403 || status == 429, RejectedBeforeInference(c))
		ClearRejection(c)
		require.False(t, RejectedBeforeInference(c))
		recordAdmissionRejection(c, nil)
		require.False(t, RejectedBeforeInference(c))
	}
}
