package replicate

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/render"
	commonsse "github.com/songquanpeng/one-api/common/sse"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

func ChatHandler(c *gin.Context, resp *http.Response) (
	srvErr *model.ErrorWithStatusCode, usage *model.Usage) {
	ctxMeta := meta.GetByContext(c)
	gmw.GetLogger(c).Debug("replicate.ChatHandler",
		zap.Int("status_code", resp.StatusCode),
		zap.Bool("is_stream", ctxMeta.IsStream),
	)

	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		return openai.ErrorWrapper(
				errors.Errorf("bad_status_code [%d]%s", resp.StatusCode, string(payload)),
				"bad_status_code", http.StatusInternalServerError),
			nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError), nil
	}

	respData := new(ChatResponse)
	if err = json.Unmarshal(respBody, respData); err != nil {
		return openai.ErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	for {
		err = func() error {
			// get task
			taskReq, err := http.NewRequestWithContext(gmw.Ctx(c),
				http.MethodGet, respData.URLs.Get, nil)
			if err != nil {
				return errors.Wrap(err, "new request")
			}

			taskReq.Header.Set("Authorization", "Bearer "+ctxMeta.APIKey)
			taskResp, err := http.DefaultClient.Do(taskReq)
			if err != nil {
				return errors.Wrap(err, "get task")
			}
			defer taskResp.Body.Close()

			if taskResp.StatusCode != http.StatusOK {
				payload, _ := io.ReadAll(taskResp.Body)
				return errors.Errorf("bad status code [%d]%s",
					taskResp.StatusCode, string(payload))
			}

			taskBody, err := io.ReadAll(taskResp.Body)
			if err != nil {
				return errors.Wrap(err, "read task response")
			}

			taskData := new(ChatResponse)
			if err = json.Unmarshal(taskBody, taskData); err != nil {
				return errors.Wrap(err, "decode task response")
			}

			gmw.GetLogger(c).Debug("replicate task status",
				zap.String("id", taskData.ID),
				zap.String("status", taskData.Status),
			)

			switch taskData.Status {
			case "succeeded":
			case "failed", "canceled":
				return errors.Errorf("task failed, [%s]%s", taskData.Status, taskData.Error)
			default:
				time.Sleep(time.Second * 3)
				return errNextLoop
			}

			if ctxMeta.IsStream {
				if taskData.URLs.Stream == "" {
					return errors.New("stream url is empty")
				}

				// request stream url
				responseText, err := chatStreamHandler(c, taskData.URLs.Stream)
				if err != nil {
					return errors.Wrap(err, "chat stream handler")
				}

				usage = openai.ResponseText2Usage(responseText,
					ctxMeta.ActualModelName, ctxMeta.PromptTokens)
				return nil
			}

			// Non-stream mode
			output, err := taskData.GetOutput()
			if err != nil {
				return errors.Wrap(err, "get output")
			}
			responseText := strings.Join(output, "")

			// Construct OpenAI response
			openaiResp := openai.TextResponse{
				Id:      taskData.ID,
				Object:  "chat.completion",
				Created: taskData.CreatedAt.Unix(),
				Model:   ctxMeta.ActualModelName,
				Choices: []openai.TextResponseChoice{
					{
						Index: 0,
						Message: model.Message{
							Role:    "assistant",
							Content: responseText,
						},
						FinishReason: "stop",
					},
				},
			}

			// Calculate usage
			usage = openai.ResponseText2Usage(responseText, ctxMeta.ActualModelName, ctxMeta.PromptTokens)
			openaiResp.Usage = *usage

			c.JSON(http.StatusOK, openaiResp)
			return nil
		}()
		if err != nil {
			if errors.Is(err, errNextLoop) {
				continue
			}

			return openai.ErrorWrapper(err, "chat_task_failed", http.StatusInternalServerError), nil
		}

		break
	}

	return nil, usage
}

const (
	eventPrefix = "event: "
	dataPrefix  = "data: "
	done        = "[DONE]"
)

func chatStreamHandler(c *gin.Context, streamUrl string) (responseText string, err error) {
	// request stream endpoint
	streamReq, err := http.NewRequestWithContext(gmw.Ctx(c), http.MethodGet, streamUrl, nil)
	if err != nil {
		return "", errors.Wrap(err, "new request to stream")
	}

	streamReq.Header.Set("Authorization", "Bearer "+meta.GetByContext(c).APIKey)
	streamReq.Header.Set("Accept", "text/event-stream")
	streamReq.Header.Set("Cache-Control", "no-store")

	resp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		return "", errors.Wrap(err, "do request to stream")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return "", errors.Errorf("bad status code [%d]%s", resp.StatusCode, string(payload))
	}

	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	common.SetEventStreamHeaders(c)
	doneRendered := false
	pendingEvent := ""
	for {
		line, readErr := lineReader.Next()
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}

			return "", errors.Wrap(readErr, "read stream line")
		}

		if line.Oversized {
			payloadBytes, readErr := io.ReadAll(line.Large)
			if readErr != nil {
				return "", errors.Wrap(readErr, "read oversized stream payload")
			}

			data := string(payloadBytes)
			switch pendingEvent {
			case "output":
				render.StringData(c, data)
				responseText += data
			case "done":
				render.Done(c)
				doneRendered = true
				return responseText, nil
			}
			continue
		}

		lineText := strings.TrimSpace(line.Text())
		if lineText == "" {
			pendingEvent = ""
			continue
		}

		// Handle comments starting with ':'
		if strings.HasPrefix(lineText, ":") {
			continue
		}

		// Parse SSE fields
		if strings.HasPrefix(lineText, eventPrefix) {
			pendingEvent = strings.TrimSpace(lineText[len(eventPrefix):])
			continue
		}

		if strings.HasPrefix(lineText, dataPrefix) {
			data := lineText[len(dataPrefix):]
			switch pendingEvent {
			case "output":
				render.StringData(c, data)
				responseText += data
			case "done":
				render.Done(c)
				doneRendered = true
				return responseText, nil
			}
		}
	}

	if !doneRendered {
		render.Done(c)
	}

	return responseText, nil
}
