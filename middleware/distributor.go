package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	gutils "github.com/Laisky/go-utils/v5"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
)

type ModelRequest struct {
	Model string `json:"model" form:"model"`
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		lg := gmw.GetLogger(c)
		userId := c.GetInt(ctxkey.Id)
		userGroup, _ := model.CacheGetUserGroup(userId)
		c.Set(ctxkey.Group, userGroup)
		var requestModel string
		var channel *model.Channel
		channelId := c.GetInt(ctxkey.SpecificChannelId)
		if channelId != 0 {
			var err error
			channel, err = model.GetChannelById(channelId, true)
			if err != nil {
				AbortWithError(c, http.StatusBadRequest, errors.New("Invalid Channel Id"))
				return
			}
			if channel.Status != model.ChannelStatusEnabled {
				AbortWithError(c, http.StatusForbidden, errors.New("The channel has been disabled"))
				return
			}
			requestModel = c.GetString(ctxkey.RequestModel)
			if requestModel != "" && !channel.SupportsModel(requestModel) {
				AbortWithError(c, http.StatusBadRequest,
					errors.Errorf("Channel #%d does not support the requested model: %s", channelId, requestModel))
				return
			}
		} else {
			requestModel = c.GetString(ctxkey.RequestModel)
			selectChannel := func(ignoreFirstPriority bool, exclude map[int]bool) (*model.Channel, error) {
				for {
					candidate, err := model.CacheGetRandomSatisfiedChannelExcluding(userGroup, requestModel, ignoreFirstPriority, exclude, false)
					if err != nil {
						return nil, errors.Wrap(err, "select channel from cache")
					}
					if requestModel == "" || candidate.SupportsModel(requestModel) {
						return candidate, nil
					}
					exclude[candidate.Id] = true
					lg.Warn("channel skipped - does not support requested model",
						zap.Int("channel_id", candidate.Id),
						zap.String("channel_name", candidate.Name),
						zap.String("requested_model", requestModel))
				}
			}

			exclude := make(map[int]bool)
			var err error
			channel, err = selectChannel(false, exclude)
			if err != nil {
				lg.Info(fmt.Sprintf("No highest priority channels available for model %s in group %s, trying lower priority channels", requestModel, userGroup))
				channel, err = selectChannel(true, exclude)
				if err != nil {
					message := fmt.Sprintf("No available channels for Model %s under Group %s", requestModel, userGroup)
					AbortWithError(c, http.StatusServiceUnavailable, errors.New(message))
					return
				}
			}
		}
		lg.Debug(fmt.Sprintf("user id %d, user group: %s, request model: %s, using channel #%d", userId, userGroup, requestModel, channel.Id))
		SetupContextForSelectedChannel(c, channel, requestModel)
		c.Next()
	}
}

func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string) {
	lg := gmw.GetLogger(c)
	// one channel could relates to multiple groups,
	// and each groud has individual ratio,
	// set minimal group ratio as channel_ratio
	var minimalRatio float64 = -1
	for grp := range strings.SplitSeq(channel.Group, ",") {
		v := ratio.GetGroupRatio(grp)
		if minimalRatio < 0 || v < minimalRatio {
			minimalRatio = v
		}
	}
	lg.Info(fmt.Sprintf("set channel %s ratio to %f", channel.Name, minimalRatio))
	c.Set(ctxkey.ChannelRatio, minimalRatio)
	c.Set(ctxkey.ChannelModel, channel)

	// generate an unique cost id for each request
	if _, ok := c.Get(ctxkey.RequestId); !ok {
		c.Set(ctxkey.RequestId, gutils.UUID7())
	}

	c.Set(ctxkey.Channel, channel.Type)
	c.Set(ctxkey.ChannelId, channel.Id)
	c.Set(ctxkey.ChannelName, channel.Name)
	c.Set(ctxkey.ContentType, c.Request.Header.Get("Content-Type"))
	if channel.SystemPrompt != nil && *channel.SystemPrompt != "" {
		c.Set(ctxkey.SystemPrompt, *channel.SystemPrompt)
	}
	c.Set(ctxkey.ModelMapping, channel.GetModelMapping())
	c.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", channel.Key))
	c.Set(ctxkey.BaseURL, channel.GetBaseURL())
	if channel.RateLimit != nil {
		c.Set(ctxkey.RateLimit, *channel.RateLimit)
	} else {
		c.Set(ctxkey.RateLimit, 0)
	}

	cfg, _ := channel.LoadConfig()
	// this is for backward compatibility
	if channel.Other != nil {
		switch channel.Type {
		case channeltype.Azure:
			if cfg.APIVersion == "" {
				cfg.APIVersion = *channel.Other
			}
		case channeltype.Xunfei:
			if cfg.APIVersion == "" {
				cfg.APIVersion = *channel.Other
			}
		case channeltype.Gemini:
			if cfg.APIVersion == "" {
				cfg.APIVersion = *channel.Other
			}
		case channeltype.AIProxyLibrary:
			if cfg.LibraryID == "" {
				cfg.LibraryID = *channel.Other
			}
		case channeltype.Ali:
			if cfg.Plugin == "" {
				cfg.Plugin = *channel.Other
			}
		}
	}
	c.Set(ctxkey.Config, cfg)
}
