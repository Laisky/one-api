package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/replicate"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func getImageRequest(c *gin.Context, _ int) (*relaymodel.ImageRequest, error) {
	imageRequest := &relaymodel.ImageRequest{}
	err := common.UnmarshalBodyReusable(c, imageRequest)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if imageRequest.N == 0 {
		imageRequest.N = 1
	}

	if imageRequest.Size == "" {
		switch imageRequest.Model {
		case "dall-e-2", "dall-e-3":
			imageRequest.Size = "1024x1024"
		case "gpt-image-1":
			imageRequest.Size = "1024x1536"
		}
	}

	if imageRequest.Model == "" {
		imageRequest.Model = "dall-e-2"
	}

	if imageRequest.Quality == "" {
		switch imageRequest.Model {
		case "dall-e-2":
			imageRequest.Quality = "standard"
		case "dall-e-3":
			imageRequest.Quality = "auto"
		case "gpt-image-1":
			imageRequest.Quality = "high"
		}
	}

	if imageRequest.Model == "gpt-image-1" {
		imageRequest.ResponseFormat = nil
	}

	return imageRequest, nil
}

func isValidImageSize(model string, size string) bool {
	if model == "cogview-3" || billingratio.ImageSizeRatios[model] == nil {
		return true
	}
	_, ok := billingratio.ImageSizeRatios[model][size]
	return ok
}

func isValidImagePromptLength(model string, promptLength int) bool {
	maxPromptLength, ok := billingratio.ImagePromptLengthLimitations[model]
	return !ok || promptLength <= maxPromptLength
}

func isWithinRange(element string, value int) bool {
	amounts, ok := billingratio.ImageGenerationAmounts[element]
	return !ok || (value >= amounts[0] && value <= amounts[1])
}

func getImageSizeRatio(model string, size string) float64 {
	if ratio, ok := billingratio.ImageSizeRatios[model][size]; ok {
		return ratio
	}
	return 1
}

func validateImageRequest(imageRequest *relaymodel.ImageRequest, _ *metalib.Meta) *relaymodel.ErrorWithStatusCode {
	// check prompt length
	if imageRequest.Prompt == "" {
		return openai.ErrorWrapper(errors.New("prompt is required"), "prompt_missing", http.StatusBadRequest)
	}

	// model validation
	if !isValidImageSize(imageRequest.Model, imageRequest.Size) {
		return openai.ErrorWrapper(errors.New("size not supported for this image model"), "size_not_supported", http.StatusBadRequest)
	}

	if !isValidImagePromptLength(imageRequest.Model, len(imageRequest.Prompt)) {
		return openai.ErrorWrapper(errors.New("prompt is too long"), "prompt_too_long", http.StatusBadRequest)
	}

	// Number of generated images validation
	if !isWithinRange(imageRequest.Model, imageRequest.N) {
		return openai.ErrorWrapper(errors.New("invalid value of n"), "n_not_within_range", http.StatusBadRequest)
	}
	return nil
}

func getImageCostRatio(imageRequest *relaymodel.ImageRequest) (float64, error) {
	if imageRequest == nil {
		return 0, errors.New("imageRequest is nil")
	}

	imageCostRatio := getImageSizeRatio(imageRequest.Model, imageRequest.Size)
	switch imageRequest.Quality {
	case "hd":
		switch imageRequest.Model {
		case "dall-e-3":
			switch imageRequest.Size {
			case "1024x1024":
				imageCostRatio *= 2
			default:
				imageCostRatio *= 1.5
			}
		}
	case "low":
		switch imageRequest.Model {
		case "gpt-image-1":
			switch imageRequest.Size {
			case "1024x1024":
				imageCostRatio = 1
			case "1024x1536",
				"1536x1024":
				imageCostRatio = 16 / 11
			default:
				return 0, errors.New("invalid size for gpt-image-1, should be 1024x1024/1024x1536/1536x1024")
			}
		}
	case "medium":
		switch imageRequest.Model {
		case "gpt-image-1":
			switch imageRequest.Size {
			case "1024x1024":
				imageCostRatio = 42 / 11
			case "1024x1536",
				"1536x1024":
				imageCostRatio = 63 / 11
			default:
				return 0, errors.New("invalid size for gpt-image-1, should be 1024x1024/1024x1536/1536x1024")
			}
		}
	case "high", "auto", "":
		switch imageRequest.Model {
		case "gpt-image-1":
			switch imageRequest.Size {
			case "1024x1024":
				imageCostRatio = 167 / 11
			case "1024x1536",
				"1536x1024",
				"":
				imageCostRatio = 250 / 11
			default:
				return 0, errors.New("invalid size for gpt-image-1, should be 1024x1024/1024x1536/1536x1024")
			}
		}
	default:
		return 0, errors.New("invalid quality, should be hd/low/medium/high/auto")
	}

	if imageCostRatio <= 0 {
		imageCostRatio = 1
	}

	return imageCostRatio, nil
}

func RelayImageHelper(c *gin.Context, relayMode int) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	meta := metalib.GetByContext(c)
	imageRequest, err := getImageRequest(c, meta.Mode)
	if err != nil {
		logger.Errorf(ctx, "getImageRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "invalid_image_request", http.StatusBadRequest)
	}

	// map model name
	var isModelMapped bool
	meta.OriginModelName = imageRequest.Model
	imageRequest.Model = meta.ActualModelName
	isModelMapped = meta.OriginModelName != meta.ActualModelName
	meta.ActualModelName = imageRequest.Model
	metalib.Set2Context(c, meta)

	// model validation
	bizErr := validateImageRequest(imageRequest, meta)
	if bizErr != nil {
		return bizErr
	}

	imageCostRatio, err := getImageCostRatio(imageRequest)
	if err != nil {
		return openai.ErrorWrapper(err, "get_image_cost_ratio_failed", http.StatusInternalServerError)
	}

	imageModel := imageRequest.Model
	// Convert the original image model
	imageRequest.Model = metalib.GetMappedModelName(imageRequest.Model, billingratio.ImageOriginModelName)
	c.Set(ctxkey.ResponseFormat, imageRequest.ResponseFormat)

	var requestBody io.Reader
	if strings.ToLower(c.GetString(ctxkey.ContentType)) == "application/json" &&
		isModelMapped || meta.ChannelType == channeltype.Azure { // make Azure channel request body
		jsonStr, err := json.Marshal(imageRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	} else {
		requestBody = c.Request.Body
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(fmt.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	// these adaptors need to convert the request
	switch meta.ChannelType {
	case channeltype.Zhipu,
		channeltype.Ali,
		channeltype.VertextAI,
		channeltype.Baidu:
		finalRequest, err := adaptor.ConvertImageRequest(c, imageRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "convert_image_request_failed", http.StatusInternalServerError)
		}
		jsonStr, err := json.Marshal(finalRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	case channeltype.Replicate:
		finalRequest, err := replicate.ConvertImageRequest(c, imageRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "convert_image_request_failed", http.StatusInternalServerError)
		}
		jsonStr, err := json.Marshal(finalRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	case channeltype.OpenAI:
		if meta.Mode != relaymode.ImagesEdits {
			jsonStr, err := json.Marshal(imageRequest)
			if err != nil {
				return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
			}

			requestBody = bytes.NewBuffer(jsonStr)
		}
	}

	// get channel-specific pricing if available
	var channelModelRatio map[string]float64
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			// Get from unified ModelConfigs only (after migration)
			channelModelRatio = channel.GetModelRatioFromConfigs()
		}
	}

	modelRatio := billingratio.GetModelRatioWithChannel(imageModel, meta.ChannelType, channelModelRatio)
	// groupRatio := billingratio.GetGroupRatio(meta.Group)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	ratio := modelRatio * groupRatio
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)

	var usedQuota int64
	switch meta.ChannelType {
	case channeltype.Replicate:
		// replicate always return 1 image
		usedQuota = int64(ratio*imageCostRatio) * 1000
	default:
		usedQuota = int64(ratio*imageCostRatio) * int64(imageRequest.N) * 1000
	}

	if userQuota < usedQuota {
		return openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}

	// do request
	resp, err := adaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		logger.Errorf(ctx, "DoRequest failed: %s", err.Error())
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	var promptTokens, completionTokens int
	defer func(ctx context.Context) {
		if resp != nil &&
			resp.StatusCode != http.StatusCreated && // replicate returns 201
			resp.StatusCode != http.StatusOK {
			return
		}

		err := model.PostConsumeTokenQuota(meta.TokenId, usedQuota)
		if err != nil {
			logger.SysError("error consuming token remain quota: " + err.Error())
		}
		err = model.CacheUpdateUserQuota(ctx, meta.UserId)
		if err != nil {
			logger.SysError("error update user quota cache: " + err.Error())
		}
		if usedQuota >= 0 {
			tokenName := c.GetString(ctxkey.TokenName)
			logContent := fmt.Sprintf("model rate %.2f, group rate %.2f, num %d",
				modelRatio, groupRatio, imageRequest.N)
			model.RecordConsumeLog(ctx, &model.Log{
				UserId:           meta.UserId,
				ChannelId:        meta.ChannelId,
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				ModelName:        imageRequest.Model,
				TokenName:        tokenName,
				Quota:            int(usedQuota),
				Content:          logContent,
				ElapsedTime:      helper.CalcElapsedTime(meta.StartTime),
			})
			model.UpdateUserUsedQuotaAndRequestCount(meta.UserId, usedQuota)
			channelId := c.GetInt(ctxkey.ChannelId)
			model.UpdateChannelUsedQuota(channelId, usedQuota)

			// also update user request cost
			docu := model.NewUserRequestCost(
				c.GetInt(ctxkey.Id),
				c.GetString(ctxkey.RequestId),
				usedQuota,
			)
			if err = docu.Insert(); err != nil {
				logger.Errorf(c, "insert user request cost failed: %+v", err)
			}
		}
	}(c.Request.Context())

	// do response
	usage, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		logger.Errorf(ctx, "respErr is not nil: %+v", respErr)
		return respErr
	}

	if usage != nil {
		promptTokens = usage.PromptTokens
		completionTokens = usage.CompletionTokens

		switch meta.ActualModelName {
		case "gpt-image-1":
			textQuota := int64(math.Ceil(float64(usage.PromptTokensDetails.TextTokens) * 10 * billingratio.MilliTokensUsd))
			imageQuota := int64(math.Ceil(float64(usage.PromptTokensDetails.ImageTokens) * 10 * billingratio.MilliTokensUsd))
			usedQuota += textQuota + imageQuota
		}
	}

	return nil
}
