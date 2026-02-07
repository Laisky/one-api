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
)

// TestResolvePricingAdaptor_UsesAPITypeFirst verifies pricing adaptor resolution uses APIType as the primary key.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestResolvePricingAdaptor_UsesAPITypeFirst(t *testing.T) {
	meta := &metalib.Meta{
		APIType:     apitype.OpenAI,
		ChannelType: channeltype.OpenAI, // this value must not be used for adaptor resolution
	}

	pricingAdaptor := resolvePricingAdaptor(meta)
	require.NotNil(t, pricingAdaptor, "expected non-nil pricing adaptor")

	_, hasOpenAIModel := pricingAdaptor.GetDefaultModelPricing()["gpt-4o"]
	require.True(t, hasOpenAIModel, "expected OpenAI pricing adaptor resolved from APIType")
}

// TestOutputBillingContextFromRequest_UsesAPITypeAdaptor verifies output billing context inherits APIType-based adaptor resolution.
// Parameters: t is the current test handle.
// Returns: nothing; assertions fail the test on mismatch.
func TestOutputBillingContextFromRequest_UsesAPITypeAdaptor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Set(ctxkey.ChannelRatio, 1.0)

	meta := &metalib.Meta{
		ActualModelName: "gpt-4o",
		APIType:         apitype.OpenAI,
		ChannelType:     channeltype.OpenAI,
	}

	billingCtx, ok := outputBillingContextFromRequest(c, meta)
	require.True(t, ok, "expected output billing context to be resolved")
	require.NotNil(t, billingCtx.PricingAdaptor, "expected pricing adaptor in output billing context")

	_, hasOpenAIModel := billingCtx.PricingAdaptor.GetDefaultModelPricing()["gpt-4o"]
	require.True(t, hasOpenAIModel, "expected OpenAI pricing adaptor in output billing context")
}
