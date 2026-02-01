package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// TestApplyOutputImageCharges verifies per-image quota is added for Gemini image outputs.
func TestApplyOutputImageCharges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.OutputImageCount, 1)

	meta := &metalib.Meta{
		ActualModelName: "gemini-2.5-flash-image-preview",
		ChannelType:     channeltype.VertextAI,
		APIType:         apitype.VertexAI,
		PromptTokens:    12,
	}

	usage := &relaymodel.Usage{
		PromptTokens:     12,
		CompletionTokens: 0,
		TotalTokens:      12,
	}

	applyOutputImageCharges(c, &usage, meta)

	expected := calculateImageBaseQuota(0.039, 0, 1.0, 1.0, 1)
	require.Equal(t, expected, usage.ToolsCost)
}
