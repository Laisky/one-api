package openai

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/alibailian"
	"github.com/songquanpeng/one-api/relay/adaptor/baiduv2"
	"github.com/songquanpeng/one-api/relay/adaptor/doubao"
	"github.com/songquanpeng/one-api/relay/adaptor/geminiv2"
	"github.com/songquanpeng/one-api/relay/adaptor/minimax"
	"github.com/songquanpeng/one-api/relay/adaptor/novita"
	"github.com/songquanpeng/one-api/relay/adaptor/openrouter"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
	ChannelType int
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.ChannelType = meta.ChannelType
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	switch meta.ChannelType {
	case channeltype.Azure:
		defaultVersion := meta.Config.APIVersion

		// https://learn.microsoft.com/en-us/azure/ai-services/openai/how-to/reasoning?tabs=python#api--feature-support
		if strings.HasPrefix(meta.ActualModelName, "o1") ||
			strings.HasPrefix(meta.ActualModelName, "o3") {
			defaultVersion = "2024-12-01-preview"
		}

		if meta.Mode == relaymode.ImagesGenerations {
			// https://learn.microsoft.com/en-us/azure/ai-services/openai/dall-e-quickstart?tabs=dalle3%2Ccommand-line&pivots=rest-api
			// https://{resource_name}.openai.azure.com/openai/deployments/dall-e-3/images/generations?api-version=2024-03-01-preview
			fullRequestURL := fmt.Sprintf("%s/openai/deployments/%s/images/generations?api-version=%s", meta.BaseURL, meta.ActualModelName, defaultVersion)
			return fullRequestURL, nil
		}

		// https://learn.microsoft.com/en-us/azure/cognitive-services/openai/chatgpt-quickstart?pivots=rest-api&tabs=command-line#rest-api
		requestURL := strings.Split(meta.RequestURLPath, "?")[0]
		requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, defaultVersion)
		task := strings.TrimPrefix(requestURL, "/v1/")
		model_ := meta.ActualModelName
		model_ = strings.Replace(model_, ".", "", -1)
		//https://github.com/songquanpeng/one-api/issues/1191
		// {your endpoint}/openai/deployments/{your azure_model}/chat/completions?api-version={api_version}
		requestURL = fmt.Sprintf("/openai/deployments/%s/%s", model_, task)
		return GetFullRequestURL(meta.BaseURL, requestURL, meta.ChannelType), nil
	case channeltype.Minimax:
		return minimax.GetRequestURL(meta)
	case channeltype.Doubao:
		return doubao.GetRequestURL(meta)
	case channeltype.Novita:
		return novita.GetRequestURL(meta)
	case channeltype.BaiduV2:
		return baiduv2.GetRequestURL(meta)
	case channeltype.AliBailian:
		return alibailian.GetRequestURL(meta)
	case channeltype.GeminiOpenAICompatible:
		return geminiv2.GetRequestURL(meta)
	default:
		return GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	if meta.ChannelType == channeltype.Azure {
		req.Header.Set("api-key", meta.APIKey)
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	if meta.ChannelType == channeltype.OpenRouter {
		req.Header.Set("HTTP-Referer", "https://github.com/Laisky/one-api")
		req.Header.Set("X-Title", "One API")
	}
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	meta := meta.GetByContext(c)
	switch meta.ChannelType {
	case channeltype.OpenRouter:
		includeReasoning := true
		request.IncludeReasoning = &includeReasoning
		if request.Provider == nil || request.Provider.Sort == "" &&
			config.OpenrouterProviderSort != "" {
			if request.Provider == nil {
				request.Provider = &openrouter.RequestProvider{}
			}

			request.Provider.Sort = config.OpenrouterProviderSort
		}
	default:
	}

	if request.Stream && !config.EnforceIncludeUsage {
		logger.Warn(c.Request.Context(),
			"please set ENFORCE_INCLUDE_USAGE=true to ensure accurate billing in stream mode")
	}

	if config.EnforceIncludeUsage && request.Stream {
		// always return usage in stream mode
		if request.StreamOptions == nil {
			request.StreamOptions = &model.StreamOptions{}
		}
		request.StreamOptions.IncludeUsage = true
	}

	// o1/o3/o4 do not support system prompt/max_tokens/temperature
	if strings.HasPrefix(meta.ActualModelName, "o") {
		temperature := float64(1)
		request.Temperature = &temperature // Only the default (1) value is supported
		request.MaxTokens = 0
		request.TopP = nil
		if request.ReasoningEffort == nil {
			effortHigh := "high"
			request.ReasoningEffort = &effortHigh
		}

		request.Messages = func(raw []model.Message) (filtered []model.Message) {
			for i := range raw {
				if raw[i].Role != "system" {
					filtered = append(filtered, raw[i])
				}
			}

			return
		}(request.Messages)
	}

	// web search do not support system prompt/max_tokens/temperature
	if strings.HasSuffix(meta.ActualModelName, "-search") {
		request.Temperature = nil
		request.TopP = nil
		request.PresencePenalty = nil
		request.N = nil
		request.FrequencyPenalty = nil
	}

	if request.Stream && !config.EnforceIncludeUsage &&
		strings.HasSuffix(request.Model, "-audio") {
		// TODO: Since it is not clear how to implement billing in stream mode,
		// it is temporarily not supported
		return nil, errors.New("set ENFORCE_INCLUDE_USAGE=true to enable stream mode for gpt-4o-audio")
	}

	return request, nil
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context,
	meta *meta.Meta,
	requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context,
	resp *http.Response,
	meta *meta.Meta) (usage *model.Usage,
	err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		var responseText string
		err, responseText, usage = StreamHandler(c, resp, meta.Mode)
		if usage == nil || usage.TotalTokens == 0 {
			usage = ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
		}
		if usage.TotalTokens != 0 && usage.PromptTokens == 0 { // some channels don't return prompt tokens & completion tokens
			usage.PromptTokens = meta.PromptTokens
			usage.CompletionTokens = usage.TotalTokens - meta.PromptTokens
		}
	} else {
		switch meta.Mode {
		case relaymode.ImagesGenerations,
			relaymode.ImagesEdits:
			err, usage = ImageHandler(c, resp)
		// case relaymode.ImagesEdits:
		// err, usage = ImagesEditsHandler(c, resp)
		default:
			err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}

	// -------------------------------------
	// calculate web-search tool cost
	// -------------------------------------
	if usage != nil {
		searchContextSize := "medium"
		var req *model.GeneralOpenAIRequest
		if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
			if req, ok = vi.(*model.GeneralOpenAIRequest); ok {
				if req != nil &&
					req.WebSearchOptions != nil &&
					req.WebSearchOptions.SearchContextSize != nil {
					searchContextSize = *req.WebSearchOptions.SearchContextSize
				}

				switch {
				case strings.HasPrefix(meta.ActualModelName, "gpt-4o-search"):
					switch searchContextSize {
					case "low":
						usage.ToolsCost += int64(math.Ceil(30 / 1000 * ratio.QuotaPerUsd))
					case "medium":
						usage.ToolsCost += int64(math.Ceil(35 / 1000 * ratio.QuotaPerUsd))
					case "high":
						usage.ToolsCost += int64(math.Ceil(40 / 1000 * ratio.QuotaPerUsd))
					default:
						return nil, ErrorWrapper(
							errors.Errorf("invalid search context size %q", searchContextSize),
							"invalid search context size: "+searchContextSize,
							http.StatusBadRequest)
					}
				case strings.HasPrefix(meta.ActualModelName, "gpt-4o-mini-search"):
					switch searchContextSize {
					case "low":
						usage.ToolsCost += int64(math.Ceil(25 / 1000 * ratio.QuotaPerUsd))
					case "medium":
						usage.ToolsCost += int64(math.Ceil(27.5 / 1000 * ratio.QuotaPerUsd))
					case "high":
						usage.ToolsCost += int64(math.Ceil(30 / 1000 * ratio.QuotaPerUsd))
					default:
						return nil, ErrorWrapper(
							errors.Errorf("invalid search context size %q", searchContextSize),
							"invalid search context size: "+searchContextSize,
							http.StatusBadRequest)
					}
				}
			}
		}
	}

	return
}

func (a *Adaptor) GetModelList() []string {
	_, modelList := GetCompatibleChannelMeta(a.ChannelType)
	return modelList
}

func (a *Adaptor) GetChannelName() string {
	channelName, _ := GetCompatibleChannelMeta(a.ChannelType)
	return channelName
}
