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

// VideoCostEstimator returns an exact whole-request USD quote for a normalized
// video request. The durable async controller owns quota conversion and billing;
// static VideoPricingConfig remains a static tariff type.
type VideoCostEstimator interface {
	EstimateVideoCostUSD(c *gin.Context, meta *meta.Meta, request *model.VideoRequest) (string, error)
}

// DynamicVideoPricingAdaptor identifies advertised video models whose price must
// be quoted per request rather than resolved from the static pricing catalog.
type DynamicVideoPricingAdaptor interface {
	VideoCostEstimator
	SupportsDynamicVideoPricing(modelName string) bool
}
