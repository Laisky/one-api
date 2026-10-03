package controller

import (
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// prepareImageRequest aligns provider-specific resolution with the billing size
// before validation or reservation. Parameters: request is mutated, cfg supplies
// defaults, and meta identifies the actual upstream. Returns: a client-input
// error for conflicting or unrepresentable known xAI tiers, otherwise nil.
func prepareImageRequest(request *relaymodel.ImageRequest, cfg *adaptor.ImagePricingConfig, meta *metalib.Meta) error {
	knownImagine := false
	if meta != nil && meta.ChannelType == channeltype.XAI && strings.HasPrefix(request.Model, "grok-imagine-image") {
		_, knownImagine = xai.ModelRatios[request.Model]
	}
	if knownImagine && strings.TrimSpace(request.Resolution) != "" {
		resolution := strings.ToLower(strings.TrimSpace(request.Resolution))
		var size string
		switch resolution {
		case "1k":
			size = "1024x1024"
		case "2k":
			size = "2048x2048"
		default:
			return errors.New("unsupported xAI image resolution: use 1k or 2k")
		}
		if explicit := normalizeImageSizeKey(request.Size); explicit != "" && explicit != size {
			return errors.New("image size and resolution select different billing tiers")
		}
		request.Size = size
		request.Resolution = resolution
	}
	applyImageDefaults(request, cfg)
	if knownImagine {
		// The documented wire enum is 1k/2k. Intermediate historical billing
		// rows are not a representable resolution selector; never silently
		// send the provider default while charging the requested larger tier.
		// https://docs.x.ai/developers/model-capabilities/images/generation
		switch request.Size {
		case "1024x1024", "2048x2048":
		default:
			return errors.New("xAI image size cannot be represented upstream: use 1024x1024 or 2048x2048")
		}
	}
	return nil
}

// convertImageRequestForUpstream isolates adapter mutations from billing state.
// Parameters: c is the request context, request is the normalized billing
// snapshot, and convert is the provider converter. Returns: the upstream DTO or
// conversion error. Both optional string pointers are copied independently.
func convertImageRequestForUpstream(c *gin.Context, request *relaymodel.ImageRequest, convert func(*gin.Context, *relaymodel.ImageRequest) (any, error)) (any, error) {
	if request == nil || convert == nil {
		return nil, errors.New("image request and converter are required")
	}
	private := *request
	if request.ResponseFormat != nil {
		value := *request.ResponseFormat
		private.ResponseFormat = &value
	}
	if request.ImagePrompt != nil {
		value := *request.ImagePrompt
		private.ImagePrompt = &value
	}
	return convert(c, &private)
}
