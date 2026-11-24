package controller

import (
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestParseInt32Query checks the min_context_length filter parser: missing,
// malformed, and negative values disable the filter, in-range values parse
// exactly, and a value beyond int32 is clamped to math.MaxInt32 instead of
// wrapping around to a small minimum that would admit every model.
// Parameters: t runs the cases. Returns: none.
func TestParseInt32Query(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for raw, want := range map[string]int32{
		"":            0,
		"abc":         0,
		"-5":          0,
		"128000":      128000,
		" 2147483647": math.MaxInt32,
		"4294967297":  math.MaxInt32, // wrapped to 1 before
		"-4294967297": 0,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/api/models/display?min_context_length="+url.QueryEscape(raw), nil)
		require.Equal(t, want, parseInt32Query(c, "min_context_length"), "min_context_length=%q", raw)
	}
}
