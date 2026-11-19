package controller

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// maxImageSafetyIdentifierRunes is OpenAI's documented safety_identifier
// length limit, counted in characters rather than UTF-8 bytes.
const maxImageSafetyIdentifierRunes = 128

// isOpenAIImageUpstream reports whether meta routes to an upstream that
// implements OpenAI's own image contract. Parameters: meta identifies the
// selected channel and may be nil. Returns: true only for OpenAI and Azure
// channels; OpenAI-compatible third parties keep their own contracts.
func isOpenAIImageUpstream(meta *metalib.Meta) bool {
	return meta != nil && (meta.ChannelType == channeltype.OpenAI || meta.ChannelType == channeltype.Azure)
}

// validateOpenAIImageControls enforces OpenAI's complexity, safety_identifier,
// and output_format contract before quota reservation or upstream dispatch.
// OpenAI image edits forward the raw client body, so every value checked here
// is one the upstream would otherwise receive verbatim. Parameters: req is the
// decoded request after channel model mapping and is normalized in place
// (complexity and output_format are trimmed and lowercased); meta selects the
// upstream contract. Returns: a client-facing HTTP 400 error, or nil when valid
// or when the upstream is not OpenAI or Azure.
func validateOpenAIImageControls(req *relaymodel.ImageRequest, meta *metalib.Meta) *relaymodel.ErrorWithStatusCode {
	if req == nil || !isOpenAIImageUpstream(meta) {
		return nil
	}
	// The identifier is end-user identifying: never echo or log its value.
	if utf8.RuneCountInString(req.SafetyIdentifier) > maxImageSafetyIdentifierRunes {
		return openai.ErrorWrapper(
			errors.Errorf("safety_identifier must be at most %d characters", maxImageSafetyIdentifierRunes),
			"invalid_value",
			http.StatusBadRequest,
		)
	}

	if req.Complexity != "" {
		complexity := strings.ToLower(strings.TrimSpace(req.Complexity))
		switch complexity {
		case "low", "medium", "high":
		default:
			return openai.ErrorWrapper(
				errors.New("invalid complexity: supported values are low, medium, high"),
				"invalid_value",
				http.StatusBadRequest,
			)
		}
		// Older OpenAI image models reject complexity upstream; never forward it.
		if !isGPTImage26Model(req.Model) {
			return openai.ErrorWrapper(
				errors.New("complexity is supported only by GPT Image 2.6 models (gpt-image-2.6-sunburst, gpt-image-2.6-flare)"),
				"invalid_value",
				http.StatusBadRequest,
			)
		}
		req.Complexity = complexity
	}

	if req.OutputFormat != "" && isOpenAIGPTImageModel(req.Model) {
		outputFormat := strings.ToLower(strings.TrimSpace(req.OutputFormat))
		switch outputFormat {
		case "png", "jpeg", "webp":
			req.OutputFormat = outputFormat
		default:
			// Only raster formats share the qualified image-token tariff. SVG adds
			// reference-image and authoring-model token charges, GIF billing is
			// undocumented, and any future format may bill differently: refuse
			// rather than forward work the gateway cannot price.
			return openai.ErrorWrapper(
				errors.New("unsupported output_format: the gateway bills only png, jpeg, and webp image output; SVG, GIF, and other formats are not supported"),
				"unsupported_output_format",
				http.StatusBadRequest,
			)
		}
	}
	return nil
}
