package controller

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // register GIF headers for reference measurement
	_ "image/jpeg" // register JPEG headers for reference measurement
	_ "image/png"  // register PNG headers for reference measurement
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp" // register WebP headers for reference measurement

	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

const (
	// imageReferenceHeaderReadLimit bounds the bytes read from one uploaded
	// image while decoding its header. A header beyond it is treated as
	// undecodable and billed at the tariff's conservative fallback.
	imageReferenceHeaderReadLimit = 1 << 20
	// imageReferenceTextReadLimit bounds the bytes read from a text form value
	// that may carry a reference image by URL or base64.
	imageReferenceTextReadLimit = 4 << 10
	// maxImageMegapixelReferences bounds the reference images priced for one
	// request; FLUX.2 accepts at most 8 (10 in BFL's playground).
	maxImageMegapixelReferences = 16
	// imageMegapixelReferenceFallbackReason flags a charge that billed at least
	// one reference image at the fallback because it could not be measured.
	imageMegapixelReferenceFallbackReason = "image_reference_dimensions_unknown_fallback"
)

// imageMegapixelCharge is the opt-in megapixel price of one generated image.
type imageMegapixelCharge struct {
	// Quote is the per-image price and its billed quantities.
	Quote pricing.ImageMegapixelQuote
	// EstimateReason is non-empty when a reference image was billed at the
	// conservative fallback instead of its measured size.
	EstimateReason string
}

// quoteImageMegapixelCharge prices a request whose image tariff opts into
// megapixel billing. Parameters: c is the request context, rawBody is the
// cached client body, req is the normalized request whose Size already carries
// the configured default, and cfg is the resolved image tariff. Returns: nil
// and nil when the tariff does not opt in; otherwise the per-image charge, or a
// client-facing error (503 for an unbillable tariff, 400 for a request that
// cannot be measured).
func quoteImageMegapixelCharge(c *gin.Context, rawBody []byte, req *relaymodel.ImageRequest, cfg *relayadaptor.ImagePricingConfig) (*imageMegapixelCharge, *relaymodel.ErrorWithStatusCode) {
	return quoteImageMegapixelChargeWithOptions(c, rawBody, req, cfg, pricing.ImageMegapixelQuoteOptions{})
}

// quoteImageMegapixelChargeWithOptions is quoteImageMegapixelCharge with the
// upstream host's billing rules. Parameters: c, rawBody, req and cfg are as for
// quoteImageMegapixelCharge, and opts carries the host rules. Returns: as
// quoteImageMegapixelCharge.
func quoteImageMegapixelChargeWithOptions(c *gin.Context, rawBody []byte, req *relaymodel.ImageRequest, cfg *relayadaptor.ImagePricingConfig, opts pricing.ImageMegapixelQuoteOptions) (*imageMegapixelCharge, *relaymodel.ErrorWithStatusCode) {
	if cfg == nil || cfg.Megapixel == nil {
		return nil, nil
	}
	if err := pricing.ValidateImageMegapixelPricing(cfg.Megapixel); err != nil {
		return nil, openai.ErrorWrapper(err, "image_billing_not_configured", http.StatusServiceUnavailable)
	}
	width, height, ok := parseImagePixelSize(req.Size)
	if !ok {
		return nil, openai.ErrorWrapper(errors.New("size must be WIDTHxHEIGHT in pixels for this image model"), "size_not_supported", http.StatusBadRequest)
	}

	var references []pricing.ImageDimensions
	if boundary, multipartBody := multipartBoundary(c); multipartBody {
		measured, err := measureMultipartReferenceImages(rawBody, boundary)
		if err != nil {
			return nil, openai.ErrorWrapper(err, "invalid_image_request", http.StatusBadRequest)
		}
		references = measured
	} else if req.IsEdit {
		// Reference images must be uploaded as files so their pixels can be
		// counted; a JSON or URL-encoded edit cannot be priced safely.
		return nil, openai.ErrorWrapper(errors.New("image edits for this model require a multipart/form-data upload"), "invalid_image_request", http.StatusBadRequest)
	}

	quote, err := pricing.QuoteImageMegapixelsWithOptions(cfg.Megapixel, width, height, references, opts)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "size_not_supported", http.StatusBadRequest)
	}
	charge := &imageMegapixelCharge{Quote: quote}
	if quote.UnknownReferences > 0 {
		charge.EstimateReason = imageMegapixelReferenceFallbackReason
	}
	lg := gmw.GetLogger(c)
	lg.Debug("image megapixel tariff quoted",
		zap.Int("output_width", width), zap.Int("output_height", height),
		zap.Float64("output_megapixels", quote.OutputMegapixels),
		zap.Int("references", quote.References), zap.Int("unknown_references", quote.UnknownReferences),
		zap.Float64("reference_megapixels", quote.ReferenceMegapixels),
		zap.Float64("unit_usd", quote.UnitUsd))
	return charge, nil
}

// parseImagePixelSize parses a normalized "WIDTHxHEIGHT" size.
// Parameters: size is the request size. Returns: positive dimensions and true,
// or false when size is not two positive decimal integers.
func parseImagePixelSize(size string) (int, int, bool) {
	widthText, heightText, found := strings.Cut(normalizeImageSizeKey(size), "x")
	if !found {
		return 0, 0, false
	}
	width, widthErr := strconv.Atoi(widthText)
	height, heightErr := strconv.Atoi(heightText)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

// multipartBoundary reports whether the client body is multipart/form-data.
// Parameters: c carries the client Content-Type. Returns: the boundary and true
// for a multipart body with a boundary, otherwise "" and false.
func multipartBoundary(c *gin.Context) (string, bool) {
	if c == nil || c.Request == nil {
		return "", false
	}
	mediaType, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return "", false
	}
	return params["boundary"], true
}

// isImageReferenceField reports whether a text form field may carry a
// reference image by URL or base64 instead of as an uploaded file.
// Parameters: name is the form field name. Returns: true for image fields.
func isImageReferenceField(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "image", "image[]", "images", "images[]":
		return true
	default:
		return false
	}
}

// measureMultipartReferenceImages measures every image a multipart request
// forwards upstream. Every file part counts as a reference image (including a
// mask, which is also an input image), and a non-empty text value in an image
// field counts as an unmeasurable reference. Parameters: body is the cached
// client body and boundary its multipart boundary. Returns: one entry per
// reference, or an error for a malformed body or too many references.
func measureMultipartReferenceImages(body []byte, boundary string) ([]pricing.ImageDimensions, error) {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var references []pricing.ImageDimensions
	for {
		part, err := reader.NextPart()
		// Only a bare io.EOF marks the final boundary. NextPart wraps io.EOF
		// for a body that ends without one (truncated or not multipart at
		// all), which must not be priced as reference-free.
		if err == io.EOF { //nolint:errorlint // the wrapped form is a malformed body
			return references, nil
		}
		if err != nil {
			return nil, errors.Wrap(err, "read multipart image request")
		}
		reference, isReference, err := measureMultipartPart(part)
		if closeErr := part.Close(); err == nil && closeErr != nil {
			err = errors.Wrap(closeErr, "close multipart part")
		}
		if err != nil {
			return nil, err
		}
		if !isReference {
			continue
		}
		if len(references) == maxImageMegapixelReferences {
			return nil, errors.Errorf("too many reference images: at most %d are accepted", maxImageMegapixelReferences)
		}
		references = append(references, reference)
	}
}

// measureMultipartPart measures one multipart part. Parameters: part is the
// current part. Returns: its dimensions, whether it is a reference image, and
// a read error for a text value that cannot be read.
func measureMultipartPart(part *multipart.Part) (pricing.ImageDimensions, bool, error) {
	if part.FileName() != "" {
		config, _, err := image.DecodeConfig(io.LimitReader(part, imageReferenceHeaderReadLimit))
		if err != nil || config.Width <= 0 || config.Height <= 0 {
			// Unknown formats or truncated headers are billed at the fallback.
			return pricing.ImageDimensions{}, true, nil
		}
		return pricing.ImageDimensions{Width: config.Width, Height: config.Height, Known: true}, true, nil
	}
	if !isImageReferenceField(part.FormName()) {
		return pricing.ImageDimensions{}, false, nil
	}
	value, err := io.ReadAll(io.LimitReader(part, imageReferenceTextReadLimit))
	if err != nil {
		return pricing.ImageDimensions{}, false, errors.Wrap(err, "read image form value")
	}
	return pricing.ImageDimensions{}, len(bytes.TrimSpace(value)) > 0, nil
}

// formatImageMegapixelDetail renders the billed megapixel quantities for the
// consume log. Parameters: charge may be nil. Returns: a leading-space detail
// string, or "" when the request was not megapixel priced.
func formatImageMegapixelDetail(charge *imageMegapixelCharge) string {
	if charge == nil {
		return ""
	}
	quote := charge.Quote
	detail := fmt.Sprintf(" output_mp=%s reference_mp=%s references=%d",
		strconv.FormatFloat(quote.OutputMegapixels, 'f', -1, 64),
		strconv.FormatFloat(quote.ReferenceMegapixels, 'f', -1, 64), quote.References)
	if quote.UnknownReferences > 0 {
		detail += fmt.Sprintf(" references_estimated=%d", quote.UnknownReferences)
	}
	return detail
}
