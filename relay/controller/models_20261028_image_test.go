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

// TestGeminiOctober28ImageChargeExclusionBoundary verifies no IMAGE token
// subtraction happens unless an actual positive per-render charge was posted,
// including when output is absent or the provider has no image pricing.
func TestGeminiOctober28ImageChargeExclusionBoundary(t *testing.T) {
	for _, tc := range []struct {
		name          string
		model         string
		groupRatio    float64
		imageCount    int
		completion    int
		imageTokens   int
		wantExcluded  int
		wantImageCost bool
	}{
		{"no image observed", "gemini-nano-banana-2.1", 1, 0, 1135, 1120, 0, false},
		{"image fee applied", "gemini-nano-banana-2.1", 1, 1, 1135, 1120, 1120, true},
		{"image fee applied and clamped", "gemini-nano-banana-2.1", 1, 1, 400, 1120, 400, true},
		{"no measured breakdown", "gemini-nano-banana-2.1", 1, 1, 1135, 0, 0, true},
		{"free channel no image charge", "gemini-nano-banana-2.1", 0, 1, 1135, 1120, 0, false},
		{"no image pricing", "gemini-3.8-flash", 1, 1, 1135, 1120, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.ChannelRatio, tc.groupRatio)
			c.Set(ctxkey.OutputImageCount, tc.imageCount)
			meta := &metalib.Meta{
				ActualModelName: tc.model,
				ChannelType:     channeltype.VertextAI,
				APIType:         apitype.VertexAI,
			}
			usage := &relaymodel.Usage{
				CompletionTokens: tc.completion,
				CandidateImageTokens: tc.imageTokens,
			}
			applyOutputImageCharges(c, &usage, meta)
			require.Equal(t, tc.completion, usage.CompletionTokens)
			require.Equal(t, tc.wantExcluded, usage.ImageTokensBilledSeparately)
			if tc.wantImageCost {
				require.Positive(t, usage.ToolsCost)
			} else {
				require.Zero(t, usage.ToolsCost)
			}
		})
	}
}
