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

	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/model"
)

func ImageHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	apiKey := c.Request.Header.Get("Authorization")
	apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	responseFormat := c.GetString("response_format")

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
		// Let ErrorWrapper handle the logging to avoid duplicate logging
		return openai.ErrorWrapper(errors.Errorf("ali async task failed: %s", aliTaskResponse.Message), "ali_async_task_failed", http.StatusInternalServerError), nil
	}

	aliResponse, _, err := asyncTaskWait(aliTaskResponse.Output.TaskId, apiKey)
	if err != nil {
		return openai.ErrorWrapper(err, "ali_async_task_wait_failed", http.StatusInternalServerError), nil
	}

	if aliResponse.Output.TaskStatus != "SUCCEEDED" {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  aliResponse.Output.Message,
				Type:     model.ErrorTypeAli,
				Param:    "",
				Code:     aliResponse.Output.Code,
				RawError: errors.New(aliResponse.Output.Message),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}

	fullTextResponse := responseAli2OpenAIImage(aliResponse, responseFormat)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, nil
}

func asyncTask(taskID string, key string) (*TaskResponse, error, []byte) {
	url := fmt.Sprintf("https://dashscope.aliyuncs.com/api/v1/tasks/%s", taskID)

	var aliResponse TaskResponse

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &aliResponse, err, nil
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// no request context here
		return &aliResponse, err, nil
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)

	var response TaskResponse
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		// no request context here
		return &aliResponse, err, nil
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
			return &taskResponse, responseBody, err
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
	imageResponse := openai.ImageResponse{
		Created: helper.GetTimestamp(),
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

// QwenImageSyncHandler handles responses from the qwen-image-2.0 synchronous endpoint.
func QwenImageSyncHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	responseFormat := c.GetString("response_format")

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}
	_ = resp.Body.Close()

	var qwenResp QwenImageSyncResponse
	if err := json.Unmarshal(responseBody, &qwenResp); err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	if qwenResp.Code != "" {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  qwenResp.Message,
				Type:     model.ErrorTypeAli,
				Code:     qwenResp.Code,
				RawError: errors.New(qwenResp.Message),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}

	imageResponse := openai.ImageResponse{
		Created: helper.GetTimestamp(),
	}
	for _, choice := range qwenResp.Output.Choices {
		for _, content := range choice.Message.Content {
			if content.Image == "" {
				continue
			}
			imgData := openai.ImageData{
				Url: content.Image,
			}
			if responseFormat == "b64_json" {
				raw, dlErr := getImageData(content.Image)
				if dlErr != nil {
					continue
				}
				imgData.B64Json = Base64Encode(raw)
				imgData.Url = ""
			}
			imageResponse.Data = append(imageResponse.Data, imgData)
		}
	}

	jsonResponse, err := json.Marshal(imageResponse)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, nil
}

func getImageData(url string) ([]byte, error) {
	response, err := http.Get(url)
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
