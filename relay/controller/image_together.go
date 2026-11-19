package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net/http"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/togetherai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

const (
	// maxTogetherImageSide bounds one requested output side. Together's
	// largest documented output is 6336 pixels wide (Gemini 3 Pro Image 4K);
	// the bound only keeps the billing arithmetic far from overflow.
	maxTogetherImageSide = 1 << 14
	// maxTogetherImageSteps bounds a priced step count. Together's
	// step-priced models default to 28-50 steps; the bound only keeps the
	// surcharge arithmetic finite.
	maxTogetherImageSteps = 1000
)

// togetherImageControls names the root members of a Together AI Images body
// that decide what Together renders and bills but that the shared
// ImageRequest does not decode. Validating against it rejects duplicate and
// case-folded spellings, which other JSON parsers may read differently.
type togetherImageControls struct {
	Width       json.RawMessage `json:"width"`
	Height      json.RawMessage `json:"height"`
	Steps       json.RawMessage `json:"steps"`
	AspectRatio json.RawMessage `json:"aspect_ratio"`
}

// togetherImageRequest is a Together AI image generation prepared for
// billing. Together's Images API has no size field: it renders the width and
// height it receives (https://docs.together.ai/reference/post-images-generations),
// so the gateway forwards the size it bills as width and height.
type togetherImageRequest struct {
	// fields is the client's JSON root object, forwarded with every member
	// the gateway does not price unchanged.
	fields map[string]json.RawMessage
	// width and height are the billed output size forwarded upstream; both
	// are zero when the tariff does not depend on the output size.
	width, height int
	// steps is the client's step count, or zero when absent or unpriced.
	steps int
	// rule is Together's billing rule for the model.
	rule togetherai.ImageBillingRule
}

// prepareTogetherImageRequest prices the Together AI controls of an image
// generation. When the tariff depends on the output size, the billed size is
// the client's width and height, else its size, else the configured default;
// it replaces req.Size and is forwarded as width and height. Parameters: c is
// the request context, meta selects the channel, req is the normalized
// request (defaults applied), clientSize is the size the client sent before
// defaults, and cfg is the resolved image tariff. Returns: nil and nil for
// other channels; otherwise the prepared request, or a client-facing error
// (400) for a body or control that cannot be priced.
func prepareTogetherImageRequest(c *gin.Context, meta *metalib.Meta, req *relaymodel.ImageRequest, clientSize string, cfg *relayadaptor.ImagePricingConfig) (*togetherImageRequest, *relaymodel.ErrorWithStatusCode) {
	if meta == nil || meta.ChannelType != channeltype.TogetherAI {
		return nil, nil
	}
	rawBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "get_request_body_failed", http.StatusInternalServerError)
	}
	if !common.IsJSONRequestBody(c, rawBody) {
		return nil, openai.ErrorWrapper(errors.New("image requests to Together AI must be a JSON object"), "invalid_image_request", http.StatusBadRequest)
	}
	if err := common.ValidateUnambiguousJSONRootKeys(rawBody, &togetherImageControls{}); err != nil {
		return nil, openai.ErrorWrapper(err, "invalid_image_request", http.StatusBadRequest)
	}
	var controls togetherImageControls
	if err := json.Unmarshal(rawBody, &controls); err != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(err, "decode Together AI image controls"), "invalid_image_request", http.StatusBadRequest)
	}
	together := &togetherImageRequest{rule: togetherai.ImageBillingRuleFor(req.Model)}
	if err := json.Unmarshal(rawBody, &together.fields); err != nil {
		return nil, openai.ErrorWrapper(errors.Wrap(err, "decode Together AI image request"), "invalid_image_request", http.StatusBadRequest)
	}

	if together.rule.DefaultSteps > 0 {
		steps, _, err := parseTogetherImageInt(controls.Steps, "steps", maxTogetherImageSteps)
		if err != nil {
			return nil, openai.ErrorWrapper(err, "invalid_image_request", http.StatusBadRequest)
		}
		together.steps = steps
	}
	if !imageTariffDependsOnSize(cfg) {
		return together, nil
	}
	if bizErr := together.resolveSize(req, clientSize, controls); bizErr != nil {
		return nil, bizErr
	}

	lg := gmw.GetLogger(c)
	lg.Debug("together image request priced on forwarded dimensions",
		zap.String("model", req.Model), zap.Int("width", together.width), zap.Int("height", together.height),
		zap.Int("steps", together.steps), zap.Int("default_steps", together.rule.DefaultSteps),
		zap.Bool("pro_rata_output", together.rule.ProRataOutput))
	return together, nil
}

// resolveSize settles the billed output size and stores it in req.Size and
// the receiver. Parameters: req carries the defaulted size, clientSize is the
// size the client sent, and controls are the raw Together controls. Returns:
// a client-facing 400 error, or nil.
func (r *togetherImageRequest) resolveSize(req *relaymodel.ImageRequest, clientSize string, controls togetherImageControls) *relaymodel.ErrorWithStatusCode {
	if togetherImageValuePresent(controls.AspectRatio) {
		// Together documents no output size for an aspect ratio, so it cannot
		// be priced; width and height select the same outputs exactly.
		return openai.ErrorWrapper(errors.New("aspect_ratio cannot be priced for this model; send size or width and height"), "invalid_image_request", http.StatusBadRequest)
	}
	width, hasWidth, err := parseTogetherImageInt(controls.Width, "width", maxTogetherImageSide)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_image_request", http.StatusBadRequest)
	}
	height, hasHeight, err := parseTogetherImageInt(controls.Height, "height", maxTogetherImageSide)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_image_request", http.StatusBadRequest)
	}
	if explicit := normalizeImageSizeKey(clientSize); explicit != "" && (hasWidth || hasHeight) {
		sizeWidth, sizeHeight, ok := parseImagePixelSize(explicit)
		if !ok || (hasWidth && width != sizeWidth) || (hasHeight && height != sizeHeight) {
			return openai.ErrorWrapper(errors.New("size and width/height select different output sizes"), "size_not_supported", http.StatusBadRequest)
		}
	}
	if !hasWidth || !hasHeight {
		baseWidth, baseHeight, ok := parseImagePixelSize(req.Size)
		if !ok {
			return openai.ErrorWrapper(errors.New("size must be WIDTHxHEIGHT in pixels for this image model"), "size_not_supported", http.StatusBadRequest)
		}
		if !hasWidth {
			width = baseWidth
		}
		if !hasHeight {
			height = baseHeight
		}
	}
	r.width, r.height = width, height
	req.Size = fmt.Sprintf("%dx%d", width, height)
	return nil
}

// stepMultiplier returns Together's step surcharge: "Cost = MP x Price per MP
// x (Steps / Default Steps)", applied "only if you go above the default steps"
// (https://docs.together.ai/docs/serverless/models.md). Parameters: none; a
// nil receiver is another channel. Returns: a multiplier of at least one.
func (r *togetherImageRequest) stepMultiplier() float64 {
	if r == nil || r.rule.DefaultSteps <= 0 || r.steps <= r.rule.DefaultSteps {
		return 1
	}
	return float64(r.steps) / float64(r.rule.DefaultSteps)
}

// quoteOptions returns the megapixel quote rules of the model. Parameters:
// none; a nil receiver is another channel. Returns: the options.
func (r *togetherImageRequest) quoteOptions() pricing.ImageMegapixelQuoteOptions {
	if r == nil {
		return pricing.ImageMegapixelQuoteOptions{}
	}
	return pricing.ImageMegapixelQuoteOptions{ProRataOutput: r.rule.ProRataOutput}
}

// logDetail renders the priced step count for the consume log. Parameters:
// none; a nil receiver is another channel. Returns: a leading-space detail,
// or "" when steps were not priced.
func (r *togetherImageRequest) logDetail() string {
	if r == nil || r.steps <= 0 || r.rule.DefaultSteps <= 0 {
		return ""
	}
	return fmt.Sprintf(" steps=%d default_steps=%d", r.steps, r.rule.DefaultSteps)
}

// body renders the upstream JSON body: the client's members, the routed
// model, and the billed width and height when the tariff is size-dependent.
// Parameters: model is the upstream model name. Returns: the body or a
// marshal error.
func (r *togetherImageRequest) body(model string) ([]byte, error) {
	fields := maps.Clone(r.fields)
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	encodedModel, err := json.Marshal(model)
	if err != nil {
		return nil, errors.Wrap(err, "marshal Together AI image model")
	}
	fields["model"] = encodedModel
	if r.width > 0 && r.height > 0 {
		fields["width"] = json.RawMessage(strconv.Itoa(r.width))
		fields["height"] = json.RawMessage(strconv.Itoa(r.height))
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, errors.Wrap(err, "marshal Together AI image request")
	}
	return encoded, nil
}

// imageTariffDependsOnSize reports whether an image tariff prices the output
// size. Parameters: cfg may be nil. Returns: true for a megapixel tariff or a
// size-keyed multiplier table.
func imageTariffDependsOnSize(cfg *relayadaptor.ImagePricingConfig) bool {
	return cfg != nil && (cfg.Megapixel != nil || len(cfg.SizeMultipliers) > 0 || len(cfg.QualitySizeMultipliers) > 0)
}

// togetherImageValuePresent reports whether a raw JSON member carries a
// value. Parameters: raw is the member, nil when absent. Returns: false for
// an absent member, null, or an empty string.
func togetherImageValuePresent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte(`""`))
}

// parseTogetherImageInt parses a positive whole-number control. Parameters:
// raw is the member, name labels errors, and limit is the largest accepted
// value. Returns: the value and true, zero and false when absent or null, or
// an error for any other JSON type, a fraction, or a value outside 1..limit.
func parseTogetherImageInt(raw json.RawMessage, name string, limit int) (int, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, false, nil
	}
	var value float64
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return 0, false, errors.Errorf("%s must be a positive integer", name)
	}
	if value != math.Trunc(value) || value < 1 || value > float64(limit) {
		return 0, false, errors.Errorf("%s must be an integer between 1 and %d", name, limit)
	}
	return int(value), true, nil
}
