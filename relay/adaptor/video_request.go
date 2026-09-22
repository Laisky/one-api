package adaptor

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// VideoRequestPreparer normalizes a provider's video request before accounting.
// PrepareVideoRequest receives the authenticated context, mapped upstream, and
// billing projection. It must synchronize the reusable body and that projection,
// return an error for invalid inputs, and never make an upstream request.
type VideoRequestPreparer interface {
	PrepareVideoRequest(c *gin.Context, metadata *meta.Meta, request *model.VideoRequest) error
}
