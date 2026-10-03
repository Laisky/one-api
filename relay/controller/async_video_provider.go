package controller

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/asyncvideo"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// ResolveAsyncVideoProvider loads current credentials for a persisted route.
// Keys are never copied into task storage. Immutable identity/type/base fences
// reject channel reuse or migration to a different provider account endpoint.
func ResolveAsyncVideoProvider(ctx context.Context, task *model.AsyncTask) (asyncvideo.Provider, *meta.Meta, error) {
	if model.DB == nil || task == nil {
		return nil, nil, errors.New("async video provider database unavailable")
	}
	var channel model.Channel
	if err := model.DB.WithContext(ctx).Where("id = ? AND uuid = ? AND type = ?", task.ChannelID, task.ChannelUUID, task.ChannelType).Take(&channel).Error; err != nil {
		return nil, nil, errors.Wrap(err, "load async video provider channel")
	}
	base := channel.GetBaseURL()
	if base == "" {
		base = channeltype.ChannelBaseURLs[channel.Type]
	}
	if strings.TrimRight(base, "/") != strings.TrimRight(task.BaseURL, "/") || channel.Key == "" {
		return nil, nil, errors.New("async video provider route changed")
	}
	if task.State == model.AsyncTaskSubmitting && channel.Status != model.ChannelStatusEnabled {
		return nil, nil, errors.New("async video provider channel disabled before submit")
	}
	ad := relay.GetAdaptor(channeltype.ToAPIType(channel.Type))
	provider, ok := ad.(asyncvideo.Provider)
	if !ok {
		return nil, nil, errors.New("channel has no durable async video provider")
	}
	info := &meta.Meta{Mode: relaymode.AsyncVideos, ChannelId: channel.Id, ChannelUUID: channel.UUID, ChannelType: channel.Type,
		APIType: channeltype.ToAPIType(channel.Type), APIKey: channel.Key, BaseURL: task.BaseURL,
		UserId: task.UserID, UserUUID: task.UserUUID, TokenId: task.TokenID, TokenUUID: task.TokenUUID,
		OriginModelName: task.OriginModel, ActualModelName: task.ActualModel, RequestURLPath: "/v1/async/videos"}
	ad.Init(info)
	return provider, info, nil
}
