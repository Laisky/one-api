package ali

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// ImageHandler creates a DashScope image task from resp, waits for it and
// writes the OpenAI-format image response to the client.
//
// A non-2xx task-creation answer (for example 429 Throttling.RateQuota) is the
// provider's refusal: it is returned with the provider's HTTP status and marked
// FromUpstream, so the relay applies the provider failure policy (cooldown,
// failover, channel health) and the image helper's 4xx refund. The raw body is
// never echoed; only the parsed DashScope message, code and request id are.
//
// Parameters:
//   - c: the request context; its Authorization header carries the task key.
//   - resp: the DashScope task-creation response; its body is consumed and closed.
//
// Returns:
//   - *model.ErrorWithStatusCode: the failure, or nil after a delivered response.
//   - *model.Usage: always nil; the image helper bills per image.
func ImageHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	apiKey := c.Request.Header.Get("Authorization")
	apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	responseFormat := c.GetString("response_format")

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return aliImageStatusError(resp), nil
	}

	var aliTaskResponse TaskResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &aliTaskResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	if aliTaskResponse.Message != "" {
		// A 2xx answer that carries an error message is still the provider's
		// failure. Without a task id no task exists, so no image can be billed:
		// mark the receipt rejected so the image helper refunds and may fail over.
		if aliTaskResponse.Output.TaskId == "" {
			c.Set(adaptor.ImageReceiptRejectedKey, true)
		}
		return openai.UpstreamErrorWrapper(errors.Errorf("ali async task failed: %s", aliTaskResponse.Message), "ali_async_task_failed", http.StatusInternalServerError), nil
	}

	aliResponse, _, err := asyncTaskWait(aliTaskResponse.Output.TaskId, apiKey)
	if err != nil {
		// A non-2xx poll answer is the provider's failure and keeps its status;
		// the accepted task's settlement is unchanged. Transport and read
		// failures prove nothing about the provider and stay local.
		var pollRefusal *aliTaskPollStatusError
		if errors.As(err, &pollRefusal) {
			return pollRefusal.failure, nil
		}
		return openai.ErrorWrapper(err, "ali_async_task_wait_failed", http.StatusInternalServerError), nil
	}

	if aliResponse.Output.TaskStatus != "SUCCEEDED" {
		// The task outcome is provider data. The accepted task keeps the
		// existing settlement (charged) until DashScope's billing for failed
		// tasks is verified; only the provenance is recorded here.
		return model.NewUpstreamError(resp.StatusCode, aliResponse.Output.Message, model.ErrorTypeAli, aliResponse.Output.Code), nil
	}

	fullTextResponse := responseAli2OpenAIImage(aliResponse, responseFormat)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	n, err := c.Writer.Write(jsonResponse)
	if err == nil && n != len(jsonResponse) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "deliver Ali image response"), "write_response_body_failed", http.StatusInternalServerError), nil
	}
	return nil, nil
}

// asyncTask polls the DashScope task endpoint once for taskID.
//
// A non-2xx poll answer is returned as an *aliTaskPollStatusError carrying a
// provider-marked error with the poll's status and only the parsed DashScope
// message, code and request id; the raw body is never echoed. Transport, read,
// close and decode failures are returned as plain wrapped errors.
//
// Parameters:
//   - taskID: the DashScope task id from the creation answer.
//   - key: the DashScope API key; it is sent only in the Authorization header.
//
// Returns:
//   - *TaskResponse: the decoded task status, or an empty response on failure.
//   - error: the failure, or nil.
//   - []byte: the raw 2xx poll body, or nil on failure.
func asyncTask(taskID string, key string) (*TaskResponse, error, []byte) {
	url := fmt.Sprintf("https://dashscope.aliyuncs.com/api/v1/tasks/%s", taskID)

	var aliResponse TaskResponse

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &aliResponse, errors.WithStack(err), nil
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// no request context here
		return &aliResponse, errors.WithStack(err), nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// aliImageStatusError consumes and closes the body.
		return &aliResponse, errors.WithStack(&aliTaskPollStatusError{failure: aliImageStatusError(resp)}), nil
	}

	responseBody, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return &aliResponse, errors.Wrap(readErr, "read DashScope task poll response"), nil
	}
	if closeErr != nil {
		return &aliResponse, errors.Wrap(closeErr, "close DashScope task poll response"), nil
	}

	var response TaskResponse
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		// no request context here
		return &aliResponse, errors.WithStack(err), nil
	}

	return &response, nil, responseBody
}

func asyncTaskWait(taskID string, key string) (*TaskResponse, []byte, error) {
	waitSeconds := 2
	step := 0
	maxStep := 20

	var taskResponse TaskResponse
	var responseBody []byte

	for {
		step++
		rsp, err, body := asyncTask(taskID, key)
		responseBody = body
		if err != nil {
			return &taskResponse, responseBody, errors.Wrap(err, "ali async image task")
		}

		if rsp.Output.TaskStatus == "" {
			return &taskResponse, responseBody, nil
		}

		switch rsp.Output.TaskStatus {
		case "FAILED":
			fallthrough
		case "CANCELED":
			fallthrough
		case "SUCCEEDED":
			fallthrough
		case "UNKNOWN":
			return rsp, responseBody, nil
		}
		if step >= maxStep {
			break
		}
		time.Sleep(time.Duration(waitSeconds) * time.Second)
	}

	return nil, nil, errors.Errorf("aliAsyncTaskWait timeout")
}

func responseAli2OpenAIImage(response *TaskResponse, responseFormat string) *openai.ImageResponse {
	// Pre-size Data to a non-nil empty slice so JSON-encoded responses always emit
	// `"data":[]` instead of `null` when every upstream result fails the b64 fetch
	// or when no results are returned at all.
	imageResponse := openai.ImageResponse{
		Created: helper.GetTimestamp(),
		Data:    make([]openai.ImageData, 0, len(response.Output.Results)),
	}

	for _, data := range response.Output.Results {
		var b64Json string
		if responseFormat == "b64_json" {
			// Read the image data from data.Url and store it in b64Json
			imageData, err := getImageData(data.Url)
			if err != nil {
				// no request context here
				continue
			}

			// Convert the image data to a Base64 encoded string
			b64Json = Base64Encode(imageData)
		} else {
			// If responseFormat is not "b64_json", use data.B64Image directly
			b64Json = data.B64Image
		}

		imageResponse.Data = append(imageResponse.Data, openai.ImageData{
			Url:           data.Url,
			B64Json:       b64Json,
			RevisedPrompt: "",
		})
	}
	return &imageResponse
}

// getImageData downloads the bytes for a base64-encoded image response. The
// URL originates from the upstream Aliyun task result, NOT from the user
// request body — but the fetch goes through client.UserContentRequestHTTPClient
// anyway as defense-in-depth against MITM-injected URLs, future callers that
// pipe user-supplied URLs through this helper, and DNS-rebinding tricks. The
// hardened client rejects loopback / private / cloud-metadata / link-local
// addresses both at first dial and on every redirect target, so an SSRF
// vector cannot be reintroduced by a refactor that forgets to validate.
func getImageData(url string) ([]byte, error) {
	httpClient := client.UserContentRequestHTTPClient
	if httpClient == nil {
		// Safety net for early init paths where client.Init() has not run
		// yet (e.g. unit tests that import this package directly). Reuse
		// the default client rather than failing — the higher-level
		// callers always ensure init() ran in production.
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Get(url)
	if err != nil {
		return nil, errors.Wrapf(err, "download image from url %s", url)
	}
	defer response.Body.Close()

	imageData, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read image response body")
	}

	return imageData, nil
}

func Base64Encode(data []byte) string {
	b64Json := base64.StdEncoding.EncodeToString(data)
	return b64Json
}
