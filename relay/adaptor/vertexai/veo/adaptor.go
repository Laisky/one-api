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
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

var ModelList = []string{
	"veo-2.0-generate-001",
	"veo-3.0-generate-preview",
}

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
			req, err := http.NewRequestWithContext(c.Request.Context(),
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
		case <-c.Request.Context().Done():
			return openai.ErrorWrapper(c.Request.Context().Err(), "request_context_done_while_waiting_for_poll", http.StatusRequestTimeout)
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
