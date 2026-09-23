package adaptor

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// AsyncVideoAcceptedKey records that an upstream has accepted a paid video job.
// A downstream write failure must not refund or automatically recreate that job.
const AsyncVideoAcceptedKey = "relay.async_video_accepted"

// VideoRequestPreparer normalizes a provider's cached JSON body and billing DTO
// before admission. PrepareVideoRequest returns the number of chargeable input
// images, or an error when the request cannot be priced and forwarded safely.
type VideoRequestPreparer interface {
	PrepareVideoRequest(c *gin.Context, request *model.VideoRequest) (int, error)
}

// VideoPricingEstimator resolves request-specific video pricing before quota admission.
// Parameters: c carries the normalized request, meta identifies the upstream channel,
// and request contains the billing fields. Return values are the effective pricing
// configuration or an error when the provider cannot quote the request safely.
type VideoPricingEstimator interface {
	EstimateVideoPricing(c *gin.Context, meta *meta.Meta, request *model.VideoRequest) (*VideoPricingConfig, error)
}
