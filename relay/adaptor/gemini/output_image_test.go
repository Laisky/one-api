package gemini

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
)

// TestCountGeminiInlineImages verifies inline image parts are counted correctly.
func TestCountGeminiInlineImages(t *testing.T) {
	response := &ChatResponse{
		Candidates: []ChatCandidate{
			{
				Content: ChatContent{Parts: []Part{
					{Text: "hello"},
					{InlineData: &InlineData{MimeType: "image/png", Data: "abc"}},
				}},
			},
			{
				Content: ChatContent{Parts: []Part{
					{InlineData: &InlineData{MimeType: "image/jpeg", Data: "def"}},
				}},
			},
		},
	}

	require.Equal(t, 2, countGeminiInlineImages(response))
}

// TestRecordGeminiOutputImageCount verifies output image counts are aggregated in context.
func TestRecordGeminiOutputImageCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)

	recordGeminiOutputImageCount(c, 2)
	recordGeminiOutputImageCount(c, 1)

	raw, ok := c.Get(ctxkey.OutputImageCount)
	require.True(t, ok, "expected output image count in context")
	require.Equal(t, 3, raw.(int))
}
