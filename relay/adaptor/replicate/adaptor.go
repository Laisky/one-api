package replicate

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
	meta *meta.Meta
}

// ConvertImageRequest implements adaptor.Adaptor.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("should call replicate.ConvertImageRequest instead")
}

func ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	meta := meta.GetByContext(c)

	if request.ResponseFormat == nil || *request.ResponseFormat != "b64_json" {
		return nil, errors.New("only support b64_json response format")
	}
	if request.N != 1 && request.N != 0 {
		return nil, errors.New("only support N=1")
	}

	switch meta.Mode {
	case relaymode.ImagesGenerations:
		return convertImageCreateRequest(request)
	case relaymode.ImagesEdits:
		return convertImageRemixRequest(c)
	default:
		return nil, errors.New("not implemented")
	}
}

func convertImageCreateRequest(request *model.ImageRequest) (any, error) {
	convertedReq := DrawImageRequest{
		Input: ImageInput{
			Steps:           25,
			Prompt:          request.Prompt,
			Guidance:        3,
			Seed:            int(time.Now().UnixNano()),
			SafetyTolerance: 5,
			NImages:         1, // replicate will always return 1 image
			Width:           1440,
			Height:          1440,
			AspectRatio:     "1:1",
		},
	}

	if strings.Contains(request.Model, "flux-kontext") {
		convertedReq.Input.InputImage = request.ImagePrompt
	} else {
		convertedReq.Input.ImagePrompt = request.ImagePrompt
	}

	return convertedReq, nil
}

func convertImageRemixRequest(c *gin.Context) (any, error) {
	// recover request body
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get request body")
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

	rawReq := new(model.OpenaiImageEditRequest)
	if err := c.ShouldBind(rawReq); err != nil {
		return nil, errors.Wrap(err, "parse image edit form")
	}

	return Convert2FluxRemixRequest(rawReq)
}

// ConvertRequest converts the request to the format that the target API expects.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if !request.Stream {
		// TODO: support non-stream mode
		return nil, errors.Errorf("replicate models only support stream mode now, please set stream=true")
	}

	// Build the prompt from OpenAI messages
	var promptBuilder strings.Builder
	for _, message := range request.Messages {
		switch msgCnt := message.Content.(type) {
		case string:
			promptBuilder.WriteString(message.Role)
			promptBuilder.WriteString(": ")
			promptBuilder.WriteString(msgCnt)
			promptBuilder.WriteString("\n")
		default:
		}
	}

	replicateRequest := ReplicateChatRequest{
		Input: ChatInput{
			Prompt:           promptBuilder.String(),
			MaxTokens:        request.MaxTokens,
			Temperature:      1.0,
			TopP:             1.0,
			PresencePenalty:  0.0,
			FrequencyPenalty: 0.0,
		},
	}

	// Map optional fields
	if request.Temperature != nil {
		replicateRequest.Input.Temperature = *request.Temperature
	}
	if request.TopP != nil {
		replicateRequest.Input.TopP = *request.TopP
	}
	if request.PresencePenalty != nil {
		replicateRequest.Input.PresencePenalty = *request.PresencePenalty
	}
	if request.FrequencyPenalty != nil {
		replicateRequest.Input.FrequencyPenalty = *request.FrequencyPenalty
	}
	if request.MaxTokens > 0 {
		replicateRequest.Input.MaxTokens = request.MaxTokens
	} else if request.MaxTokens == 0 {
		replicateRequest.Input.MaxTokens = 500
	}

	return replicateRequest, nil
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.meta = meta
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	if !slices.Contains(ModelList, meta.OriginModelName) {
		return "", errors.Errorf("model %s not supported", meta.OriginModelName)
	}

	return fmt.Sprintf("https://api.replicate.com/v1/models/%s/predictions", meta.OriginModelName), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	logger.Info(c, "send request to replicate")
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	switch meta.Mode {
	case relaymode.ImagesGenerations,
		relaymode.ImagesEdits:
		err, usage = ImageHandler(c, resp)
	case relaymode.ChatCompletions:
		err, usage = ChatHandler(c, resp)
	default:
		err = openai.ErrorWrapper(errors.New("not implemented"), "not_implemented", http.StatusInternalServerError)
	}

	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "replicate"
}
