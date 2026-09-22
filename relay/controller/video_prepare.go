package controller

import (
	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// prepareVideoForBilling invokes an optional provider contract before pricing.
// Parameters: c carries the reusable body, meta has the mapped model, and request
// is the billing projection. Returns: whether the body was prepared and a wrapped
// error. Providers without this capability retain their existing request path.
func prepareVideoForBilling(c *gin.Context, meta *metalib.Meta, request *relaymodel.VideoRequest) (bool, error) {
	ad := relay.GetAdaptor(meta.APIType)
	if ad == nil {
		return false, errors.New("video adaptor is unavailable")
	}
	ad.Init(meta)
	preparer, ok := ad.(adaptor.VideoRequestPreparer)
	if !ok {
		return false, nil
	}
	if err := preparer.PrepareVideoRequest(c, meta, request); err != nil {
		return false, errors.Wrap(err, "prepare provider video request")
	}
	if raw, ok := c.Get(ctxkey.AsyncTaskRequestMetadata); ok {
		if snapshot, ok := raw.(map[string]any); ok {
			snapshot["duration_seconds"] = request.RequestedDurationSeconds()
			snapshot["resolution"] = request.RequestedResolution()
		}
	}
	return true, nil
}
