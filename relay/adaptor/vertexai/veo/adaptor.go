package veo

import (
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

var (
	veoTextImageInputs = []string{"text", "image"}
	// veoVideoOutputs is left nil because OpenRouter's listing schema enumerates
	// only "text", "image", "file" as valid output modalities; video generation
	// is captured in each model's Description field instead.
	veoVideoOutputs []string
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on VertexAI Veo pricing: https://cloud.google.com/vertex-ai/generative-ai/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// Veo 2 pricing: $0.50 per second.
	// Covers GA and preview variants that share the Veo 2 generation SKU.
	"veo-2.0-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.50),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.50),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 2 video generation on Vertex AI. Deprecated 2026-06-30; migrate to the veo-3.1 equivalent.",
	},
	"veo-2.0-generate-exp": {
		Ratio:           veoRatioFromUsdPerSecond(0.50),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.50),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 2 experimental video generation on Vertex AI. Discontinued 2026-04-02.",
	},
	"veo-2.0-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.50),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.50),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 2 preview video generation on Vertex AI. Discontinued 2026-04-02.",
	},

	// Veo 3 pricing baseline (video-only 720p/1080p): $0.20 per second.
	// Audio and 4k add-ons are provider-side options not modeled in generic OpenAI video fields yet.
	"veo-3.0-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.20),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.20),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3 video generation on Vertex AI. Deprecated 2026-06-30; migrate to the veo-3.1 equivalent.",
	},
	"veo-3.0-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.20),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.20),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3 preview video generation on Vertex AI. Discontinued 2026-04-02.",
	},

	// Veo 3 Fast pricing baseline (video-only 720p): $0.08 per second; 1080p is $0.10/sec
	// per https://cloud.google.com/vertex-ai/generative-ai/pricing. The historical entry billed a
	// flat $0.10 baseline at the with-audio rate, retained here for backward compatibility while
	// the 1080p multiplier is recorded explicitly.
	"veo-3.0-fast-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.10),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.10, map[string]float64{
			"1080p": 1.2,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3 Fast video generation on Vertex AI (1080p surcharge applies). Deprecated 2026-06-30; migrate to the veo-3.1 equivalent.",
	},
	"veo-3.0-fast-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.10),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.10, map[string]float64{
			"1080p": 1.2,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3 Fast preview video generation on Vertex AI (1080p surcharge applies). Discontinued 2026-04-02.",
	},

	// Veo 3.1 pricing baseline (video-only 720p/1080p): $0.20 per second.
	// 4k requests are billed at $0.40 per second (2x multiplier).
	"veo-3.1-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.20),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.20, map[string]float64{
			"4k": 2,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3.1 video generation on Vertex AI (4k surcharge applies).",
	},
	"veo-3.1-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.20),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.20, map[string]float64{
			"4k": 2,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3.1 preview video generation on Vertex AI (4k surcharge applies). Discontinued 2026-04-02.",
	},

	// Veo 3.1 Fast pricing baseline (video-only 720p): $0.08 per second; 1080p is $0.10/sec;
	// 4k requests are billed at $0.25 per second (video-only) per Vertex AI pricing.
	// The historical entry billed a flat $0.10 baseline at the with-audio rate; we keep the same
	// baseline for backward compatibility and adjust the 4k multiplier from 3 → 2.5 to align with
	// $0.25 video-only and $0.30 with-audio (Gemini API table) per https://cloud.google.com/vertex-ai/generative-ai/pricing.
	"veo-3.1-fast-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.10),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.10, map[string]float64{
			"1080p": 1.2,
			"4k":    3,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3.1 Fast video generation on Vertex AI (1080p and 4k surcharges apply).",
	},
	"veo-3.1-fast-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.10),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.10, map[string]float64{
			"1080p": 1.2,
			"4k":    3,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3.1 Fast preview video generation on Vertex AI (1080p and 4k surcharges apply). Discontinued 2026-04-02.",
	},

	// Veo 3.1 Lite (preview) pricing per https://ai.google.dev/gemini-api/docs/pricing:
	// $0.05 per second at 720p with audio; 1080p is $0.08/sec (1.6x). 4k unsupported.
	"veo-3.1-lite-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.05),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.05, map[string]float64{
			"1080p": 1.6,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3.1 Lite preview video generation on Vertex AI (1080p surcharge applies). Discontinued 2026-04-02.",
	},

	// Veo 3.1 Lite (GA, released 2026-04-02) pricing per https://ai.google.dev/gemini-api/docs/pricing:
	// $0.05 per second at 720p with audio; 1080p is $0.08/sec (1.6x). 4k unsupported.
	"veo-3.1-lite-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.05),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.05, map[string]float64{
			"1080p": 1.6,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3.1 Lite video generation on Vertex AI (1080p surcharge applies).",
	},
}

// veoRatioFromUsdPerSecond converts a per-second USD price into a per-token ratio.
// Billing for this adaptor reports completion tokens as durationSeconds * ratio.TokensPerSec.
func veoRatioFromUsdPerSecond(usdPerSecond float64) float64 {
	return usdPerSecond * ratio.QuotaPerUsd / float64(ratio.TokensPerSec)
}

// veoVideoPricingConfig builds video pricing metadata consumed by /v1/videos billing.
func veoVideoPricingConfig(perSecondUsd float64, multipliers ...map[string]float64) *adaptor.VideoPricingConfig {
	cfg := &adaptor.VideoPricingConfig{
		PerSecondUsd:   perSecondUsd,
		BaseResolution: "720p",
	}
	if len(multipliers) > 0 && len(multipliers[0]) > 0 {
		cfg.ResolutionMultipliers = make(map[string]float64, len(multipliers[0]))
		for k, v := range multipliers[0] {
			cfg.ResolutionMultipliers[k] = v
		}
	}
	return cfg
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

const (
	actionPredictLongRunning = ":predictLongRunning"
	actionFetchOperation     = ":fetchPredictOperation"
	defaultVideoDurationSec  = 8
)

// Adaptor is the Vertex AI Veo sub-adaptor. The zero value polls with the
// package defaults; tests may shorten the poll interval and timeout.
type Adaptor struct {
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// Init prepares the adaptor for one request; Veo keeps no per-request state.
func (a *Adaptor) Init(meta *meta.Meta) {
	// No initialization needed
}

// ConvertRequest converts the last chat message into a Veo predictLongRunning
// request. It accepts the gin context, relay mode and chat request, and returns
// the Vertex request or an error when the request has no messages.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	sampleCount := 1
	if request.N != nil && *request.N > 1 {
		sampleCount = *request.N
	}

	if len(request.Messages) == 0 {
		return nil, errors.New("messages cannot be empty")
	}

	lastMsg := request.Messages[len(request.Messages)-1]
	contents := lastMsg.ParseContent()
	var textPrompt, imgPrompt string
	for _, content := range contents {
		if content.Text != nil && *content.Text != "" {
			textPrompt = *content.Text
		}

		if content.ImageURL != nil && content.ImageURL.Url != "" {
			imgPrompt = content.ImageURL.Url
		}
	}

	convertedReq := &CreateVideoRequest{
		Instances: []CreateVideoInstance{
			{
				Prompt: textPrompt,
			},
		},
		Parameters: CreateVideoParameters{
			SampleCount: sampleCount,
		},
	}

	if imgPrompt != "" {
		convertedReq.Instances[0].Image = &CreateVideoInstanceImage{
			BytesBase64Encoded: imgPrompt,
		}
	}
	if request.Duration != nil && *request.Duration > 0 {
		convertedReq.Parameters.DurationSeconds = request.Duration
	} else {
		d := defaultVideoDurationSec
		convertedReq.Parameters.DurationSeconds = &d
	}

	return convertedReq, nil
}

// ConvertImageRequest is unsupported for Veo and always returns an error.
func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest is unsupported for Veo and always returns an error.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// VertexAI VEO doesn't support Claude Messages API directly
	return nil, errors.New("Claude Messages API not supported by VertexAI VEO adaptor")
}

// DoResponse handles the predictLongRunning response. A provider rejection is
// reported with the provider status and a sanitized message. An accepted job
// (HTTP 200) always reports its billable duration, even when reading the
// response or polling the operation fails afterwards: the provider owns the
// paid work and the relay must not resubmit it elsewhere.
// It accepts the gin context, the create response and the relay meta, and
// returns the usage and the relay error, if any.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, wrapErr *model.ErrorWithStatusCode) {
	accepted := resp.StatusCode == http.StatusOK
	limit := int64(maxErrorBodyBytes)
	if accepted {
		limit = maxCreateBodyBytes
		usage = acceptedVideoUsage(c)
	}
	respBody, tooLarge, err := readBoundedBody(resp.Body, limit)
	if err != nil {
		return usage, openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	if err = resp.Body.Close(); err != nil {
		return usage, openai.ErrorWrapper(errors.Wrap(err, "close veo create response"), "close_response_body_failed", http.StatusInternalServerError)
	}

	if !accepted {
		if tooLarge {
			respBody = nil
		}
		return nil, openai.UpstreamErrorWrapper(errors.New(upstreamErrorMessage(respBody, resp.StatusCode)), "veo_api_error", resp.StatusCode)
	}
	if tooLarge {
		return usage, openai.ErrorWrapper(errors.Errorf("veo create response exceeds %d bytes", maxCreateBodyBytes), "unmarshal_poll_response_failed", http.StatusInternalServerError)
	}

	return usage, a.pollVideoTask(c, resp, respBody)
}

// acceptedVideoUsage returns the billable usage of an accepted job: the
// requested duration, or the default duration, in completion tokens.
// It accepts the gin context carrying the converted request.
func acceptedVideoUsage(c *gin.Context) *model.Usage {
	duration := defaultVideoDurationSec
	if reqi, ok := c.Get(ctxkey.ConvertedRequest); ok {
		if convertedReq, ok := reqi.(*CreateVideoRequest); ok && convertedReq.Parameters.DurationSeconds != nil {
			duration = *convertedReq.Parameters.DurationSeconds
		}
	}
	return &model.Usage{
		CompletionTokens: duration * ratio.TokensPerSec,
		TotalTokens:      duration * ratio.TokensPerSec,
	}
}

// GetModelList returns the Veo models this sub-adaptor serves.
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName returns the sub-adaptor's channel name.
func (a *Adaptor) GetChannelName() string {
	return "vertex_ai_veo"
}
