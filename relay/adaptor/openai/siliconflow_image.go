package openai

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
)

const siliconFlowImagePayloadKey = "relay.siliconflow_image_payload"

// PrepareSiliconFlowImage resolves native size aliases before quota calculation.
// The audited contract is one text-to-image output per call. Extensions are
// preserved only when they cannot introduce additional unmetered work.
func PrepareSiliconFlowImage(c *gin.Context, request *model.ImageRequest) error {
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" || request.IsEdit {
		return errors.New("SiliconFlow images require JSON /v1/images/generations")
	}
	body, err := common.GetRequestBody(c)
	if err != nil {
		return errors.Wrap(err, "read SiliconFlow image input")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return errors.Wrap(err, "decode SiliconFlow image input")
	}
	if payload == nil {
		return errors.New("image input must be an object")
	}
	if raw, exists := payload["extra_body"]; exists {
		var extra map[string]json.RawMessage
		if err := json.Unmarshal(raw, &extra); err != nil || extra == nil {
			return errors.New("extra_body must be an object")
		}
		for key, value := range extra {
			if key == "model" || key == "prompt" {
				return errors.New("extra_body must not replace billed model or prompt")
			}
			if _, exists := payload[key]; !exists {
				payload[key] = value
			}
		}
		delete(payload, "extra_body")
	}
	for _, key := range []string{"n", "batch_size", "num_images", "num_outputs"} {
		if raw, exists := payload[key]; exists {
			var count int
			if err := json.Unmarshal(raw, &count); err != nil || count != 1 {
				return errors.New("SiliconFlow generates one image per call; use separate requests, not n/batch_size > 1")
			}
			delete(payload, key)
		}
	}
	for _, key := range []string{"image", "images", "image_url", "image_urls", "mask", "mask_url", "reference_images"} {
		if _, exists := payload[key]; exists {
			return errors.Errorf("SiliconFlow %s requires an image-editing tariff; this endpoint supports text-to-image only", key)
		}
	}
	if raw, exists := payload["seed"]; exists {
		var seed int64
		if err := json.Unmarshal(raw, &seed); err != nil || seed < 0 || seed > 9999999999 {
			return errors.New("SiliconFlow seed must be an integer between 0 and 9999999999")
		}
	}
	for _, key := range []string{"quality", "style", "resolution"} {
		if raw, exists := payload[key]; exists && string(raw) != `""` {
			return errors.Errorf("unsupported SiliconFlow image field %s; use image_size and native options", key)
		}
		delete(payload, key)
	}
	var size string
	for _, key := range []string{"size", "image_size"} {
		if raw, exists := payload[key]; exists {
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return errors.Wrapf(err, "decode image %s", key)
			}
			value = strings.TrimSpace(strings.ToLower(value))
			if size != "" && size != value {
				return errors.New("size and image_size disagree")
			}
			size = value
		}
	}
	if size != "" {
		width, height, ok := strings.Cut(size, "x")
		w, e1 := strconv.Atoi(width)
		h, e2 := strconv.Atoi(height)
		if !ok || e1 != nil || e2 != nil || w < 64 || h < 64 || w > 4096 || h > 4096 {
			return errors.New("image_size must contain dimensions between 64 and 4096")
		}
		request.Size = size
	}
	if raw, exists := payload["response_format"]; exists {
		var format string
		if err := json.Unmarshal(raw, &format); err != nil || (format != "" && format != "url") {
			return errors.New("SiliconFlow image output supports response_format=url only")
		}
	}
	delete(payload, "size")
	delete(payload, "response_format")
	request.N = 1
	c.Set(siliconFlowImagePayloadKey, payload)
	return nil
}

// ConvertSiliconFlowImage constructs the native request from the normalized
// billing snapshot and preserved extensions; no price selector can diverge.
func ConvertSiliconFlowImage(c *gin.Context, request *model.ImageRequest) (any, error) {
	raw, ok := c.Get(siliconFlowImagePayloadKey)
	if !ok {
		return nil, errors.New("SiliconFlow image input was not prepared")
	}
	original, ok := raw.(map[string]json.RawMessage)
	if !ok {
		return nil, errors.New("invalid prepared image payload")
	}
	if request.Model == "black-forest-labs/FLUX.2-pro" {
		switch request.Size {
		case "512x512", "768x1024", "1024x768", "576x1024", "1024x576":
		default:
			return nil, errors.New("unsupported FLUX.2-pro size; use 512x512, 768x1024, 1024x768, 576x1024 or 1024x576")
		}
	}
	wireModel := request.Model
	switch wireModel {
	case "black-forest-labs/FLUX.1.1-pro":
		wireModel = "black-forest-labs/FLUX-1.1-pro"
	case "Bytedance/Z-Image-Turbo":
		wireModel = "Tongyi-MAI/Z-Image-Turbo"
	}
	payload := make(map[string]json.RawMessage, len(original)+3)
	for key, value := range original {
		payload[key] = value
	}
	for key, value := range map[string]string{"model": wireModel, "prompt": request.Prompt, "image_size": request.Size} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, errors.Wrap(err, "encode SiliconFlow image field")
		}
		payload[key] = encoded
	}
	// Qwen edit variants take dimensions from the input image. A dedicated edit
	// contract is needed rather than silently applying a different billed size.
	if strings.Contains(strings.ToLower(request.Model), "image-edit") {
		return nil, errors.New("SiliconFlow Qwen Image Edit needs the native editing contract")
	}
	return payload, nil
}

// SiliconFlowImageHandler translates the native images array to OpenAI data.
// It records acceptance separately from downstream delivery so malformed/error
// responses refund, while a delivered provider image remains billable.
func SiliconFlowImageHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	const limit = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	closeErr := resp.Body.Close()
	c.Set(adaptor.ImageReceiptRejectedKey, true)
	if err != nil {
		return ErrorWrapper(errors.Wrap(err, "read SiliconFlow image response"), "invalid_image_response", http.StatusBadGateway), nil
	}
	if closeErr != nil {
		return ErrorWrapper(errors.Wrap(closeErr, "close SiliconFlow image response"), "invalid_image_response", http.StatusBadGateway), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return buildImageUpstreamError(body, resp.StatusCode), nil
	}
	if len(body) > limit {
		return ErrorWrapper(errors.New("SiliconFlow image response exceeds limit"), "invalid_image_response", http.StatusBadGateway), nil
	}
	var payload struct {
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
		Error   json.RawMessage `json:"error"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ErrorWrapper(errors.Wrap(err, "decode SiliconFlow image response"), "invalid_image_response", http.StatusBadGateway), nil
	}
	if len(payload.Images) != 1 || (len(payload.Error) != 0 && string(payload.Error) != "null") || payload.Message != "" {
		return ErrorWrapper(errors.New("SiliconFlow did not return exactly one generated image"), "invalid_image_response", http.StatusBadGateway), nil
	}
	uri, err := url.Parse(payload.Images[0].URL)
	if err != nil || uri.Host == "" || (uri.Scheme != "https" && uri.Scheme != "http") {
		return ErrorWrapper(errors.New("invalid generated image URL"), "invalid_image_response", http.StatusBadGateway), nil
	}
	data := struct {
		Created int64 `json:"created"`
		Data    []struct {
			URL string `json:"url"`
		} `json:"data"`
	}{Created: time.Now().Unix(), Data: payload.Images}
	encoded, err := json.Marshal(data)
	if err != nil {
		return ErrorWrapper(errors.Wrap(err, "encode image response"), "invalid_image_response", http.StatusInternalServerError), nil
	}
	c.Set(adaptor.ImageReceiptRejectedKey, false)
	c.Set(adaptor.ImageReceiptAcceptedKey, true)
	for _, key := range []string{"X-Siliconcloud-Trace-Id", "X-Request-Id"} {
		if value := resp.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	c.Header("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := c.Writer.Write(encoded); err != nil {
		return ErrorWrapper(errors.Wrap(err, "write image result"), "write_response_body_failed", http.StatusBadGateway), &model.Usage{}
	}
	return nil, &model.Usage{}
}
