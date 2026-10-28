package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestGeminiOctober28NanoBananaOutputBilling verifies image render counts use
// the new 1K Standard tariff rather than falling back to zero or stale prices.
func TestGeminiOctober28NanoBananaOutputBilling(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(ctxkey.ChannelRatio, 1.0)
		c.Set(ctxkey.OutputImageCount, count)
		requestMeta := &meta.Meta{
			ActualModelName: "gemini-nano-banana-2.1",
			ChannelType:     channeltype.VertextAI,
			APIType:         apitype.VertexAI,
			PromptTokens:    8,
		}
		usage := &model.Usage{PromptTokens: 8}
		applyOutputImageCharges(c, &usage, requestMeta)
		require.Equal(t,
			calculateImageBaseQuota(0.0336, 0, 1, 1, count),
			usage.ToolsCost,
			"unexpected image charge for %d outputs", count)
	}
}
