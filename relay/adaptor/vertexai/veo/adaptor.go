package veo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
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
		Description: "Google Veo 2 video generation on Vertex AI.",
	},
	"veo-2.0-generate-exp": {
		Ratio:           veoRatioFromUsdPerSecond(0.50),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.50),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 2 experimental video generation on Vertex AI.",
	},
	"veo-2.0-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.50),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.50),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 2 preview video generation on Vertex AI.",
	},

	// Veo 3 pricing baseline (video-only 720p/1080p): $0.20 per second.
	// Audio and 4k add-ons are provider-side options not modeled in generic OpenAI video fields yet.
	"veo-3.0-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.20),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.20),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3 video generation on Vertex AI.",
	},
	"veo-3.0-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.20),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.20),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3 preview video generation on Vertex AI.",
	},

	// Veo 3 Fast pricing baseline (video-only 720p/1080p): $0.10 per second.
	"veo-3.0-fast-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.10),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.10),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3 Fast video generation on Vertex AI.",
	},
	"veo-3.0-fast-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.10),
		CompletionRatio: 1,
		Video:           veoVideoPricingConfig(0.10),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3 Fast preview video generation on Vertex AI.",
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
		Description: "Google Veo 3.1 preview video generation on Vertex AI (4k surcharge applies).",
	},

	// Veo 3.1 Fast pricing baseline (video-only 720p/1080p): $0.10 per second.
	// 4k requests are billed at $0.30 per second (3x multiplier).
	"veo-3.1-fast-generate-001": {
		Ratio:           veoRatioFromUsdPerSecond(0.10),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.10, map[string]float64{
			"4k": 3,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3.1 Fast video generation on Vertex AI (4k surcharge applies).",
	},
	"veo-3.1-fast-generate-preview": {
		Ratio:           veoRatioFromUsdPerSecond(0.10),
		CompletionRatio: 1,
		Video: veoVideoPricingConfig(0.10, map[string]float64{
			"4k": 3,
		}),
		InputModalities: veoTextImageInputs, OutputModalities: veoVideoOutputs,
		Description: "Google Veo 3.1 Fast preview video generation on Vertex AI (4k surcharge applies).",
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
	pollInterval             = 5 * time.Second // Polling interval for video task status
	actionPredictLongRunning = ":predictLongRunning"
	actionFetchOperation     = ":fetchPredictOperation"
	defaultVideoDurationSec  = 8
)

type Adaptor struct {
}

func (a *Adaptor) Init(meta *meta.Meta) {
	// No initialization needed
}

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

func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// VertexAI VEO doesn't support Claude Messages API directly
	return nil, errors.New("Claude Messages API not supported by VertexAI VEO adaptor")
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, wrapErr *model.ErrorWithStatusCode) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	err = resp.Body.Close() // Close the original body
	if err != nil {
		return nil, openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, openai.ErrorWrapper(errors.New(string(respBody)), "veo_api_error", resp.StatusCode)
	}

	duration := defaultVideoDurationSec
	if reqi, ok := c.Get(ctxkey.ConvertedRequest); ok {
		if convertedReq, ok := reqi.(*CreateVideoRequest); ok && convertedReq.Parameters.DurationSeconds != nil {
			duration = *convertedReq.Parameters.DurationSeconds
		}
	}

	return &model.Usage{
		CompletionTokens: duration * ratio.TokensPerSec,
		TotalTokens:      duration * ratio.TokensPerSec,
	}, pollVideoTask(c, resp, respBody)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "vertex_ai_veo"
}

func pollVideoTask(
	c *gin.Context,
	resp *http.Response,
	respBody []byte,
) *model.ErrorWithStatusCode {
	pollTask := new(CreateVideoTaskResponse)
	if err := json.Unmarshal(respBody, pollTask); err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "unmarshal_poll_response_failed"), "unmarshal_poll_response_failed", http.StatusInternalServerError)
	}

	pollUrl := strings.ReplaceAll(resp.Request.RequestURI,
		actionPredictLongRunning, actionFetchOperation)

	pollRequestBody := PollVideoTaskRequest{
		OperationName: pollTask.Name,
	}
	pollBodyBytes, err := json.Marshal(pollRequestBody)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "marshal_poll_request_failed"), "marshal_poll_request_failed", http.StatusInternalServerError)
	}

	for {
		var videoResult *PollVideoTaskResponse
		var pollAttemptErr *model.ErrorWithStatusCode

		func() { // Anonymous function to scope defer
			req, err := http.NewRequestWithContext(gmw.Ctx(c),
				http.MethodPost, pollUrl, bytes.NewBuffer(pollBodyBytes))
			if err != nil {
				pollAttemptErr = openai.ErrorWrapper(errors.Wrap(err, "create_poll_request_failed"), "create_poll_request_failed", http.StatusInternalServerError)
				return
			}

			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				pollAttemptErr = openai.ErrorWrapper(errors.Wrap(err, "do_poll_request_failed"), "do_poll_request_failed", http.StatusServiceUnavailable)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				errMsg := fmt.Sprintf("poll_video_task_failed, status_code: %d, body: %s", resp.StatusCode, string(bodyBytes))
				pollAttemptErr = openai.ErrorWrapper(errors.New(errMsg), "poll_request_failed_status", resp.StatusCode)
				return
			}

			currentVideoResult := new(PollVideoTaskResponse)
			if err := json.NewDecoder(resp.Body).Decode(currentVideoResult); err != nil {
				pollAttemptErr = openai.ErrorWrapper(errors.Wrap(err, "unmarshal_poll_response_failed"), "unmarshal_poll_response_failed", http.StatusInternalServerError)
				return
			}
			videoResult = currentVideoResult
		}()

		if pollAttemptErr != nil {
			return pollAttemptErr
		}

		if videoResult != nil {
			if videoResult.Done {
				return convert2OpenaiResponse(c, videoResult)
			}
		}

		// Task not done, wait before next poll
		select {
		case <-time.After(pollInterval):
			// Continue to next iteration
		case <-gmw.Ctx(c).Done():
			return openai.ErrorWrapper(gmw.Ctx(c).Err(), "request_context_done_while_waiting_for_poll", http.StatusRequestTimeout)
		}
	}
}

func convert2OpenaiResponse(c *gin.Context, veoResp *PollVideoTaskResponse) *model.ErrorWithStatusCode {
	if veoResp == nil {
		return openai.ErrorWrapper(errors.New("VEO response is nil"), "veo_response_nil", http.StatusInternalServerError)
	}

	// It's assumed that this function is called only when veoResp.Done is true.
	// A check could be added:
	// if !veoResp.Done {
	//	 return openai.ErrorWrapper(errors.New("VEO task is not done"), "task_not_done", http.StatusInternalServerError)
	// }

	imageDatas := make([]openai.ImageData, 0, len(veoResp.Response.GeneratedSamples))
	for _, sample := range veoResp.Response.GeneratedSamples {
		imageData := openai.ImageData{
			Url: sample.Video.URI, // VEO provides a URI to the video.
			// B64Json and RevisedPrompt are not available from this VEO response.
		}
		imageDatas = append(imageDatas, imageData)
	}

	openaiResp := &openai.ImageResponse{
		Created: helper.GetTimestamp(),
		Data:    imageDatas,
		// Usage for video generation is not directly provided by VEO in a token-based format.
		// The openai.ImageResponse.Usage field (type openai.ImageUsage) will be its zero value.
	}

	jsonResponse, err := json.Marshal(openaiResp)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_openai_response_failed", http.StatusInternalServerError)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	if _, err := c.Writer.Write(jsonResponse); err != nil {
		// If WriteHeader has been called, an error here is harder to report to the client cleanly.
		return openai.ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError)
	}

	return nil
}
