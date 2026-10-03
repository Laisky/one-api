package model

import "github.com/Laisky/one-api/dto"

// ToResponse builds the external boundary DTO for a channel. It replaces the
// retired Channel.MarshalJSON whitelist: the internal integer id never crosses
// the API.
//
// Parameters: none (pointer receiver).
//
// Return values:
//   - dto.ChannelResponse: the UUID-only external shape. A nil receiver yields
//     the zero shape (so the channelListItem wrapper keeps its nil-channel
//     behavior).
func (channel *Channel) ToResponse() dto.ChannelResponse {
	if channel == nil {
		return dto.ChannelResponse{}
	}
	return dto.ChannelResponse{
		UUID:                   channel.UUID,
		Type:                   channel.Type,
		Key:                    channel.Key,
		Status:                 channel.Status,
		Name:                   channel.Name,
		Weight:                 channel.Weight,
		CreatedTime:            channel.CreatedTime,
		TestTime:               channel.TestTime,
		ResponseTime:           channel.ResponseTime,
		BaseURL:                channel.BaseURL,
		Other:                  channel.Other,
		Balance:                channel.Balance,
		BalanceUpdatedTime:     channel.BalanceUpdatedTime,
		Models:                 channel.Models,
		HiddenModels:           channel.HiddenModels,
		ModelConfigs:           channel.ModelConfigs,
		Group:                  channel.Group,
		UsedQuota:              channel.UsedQuota,
		ModelMapping:           channel.ModelMapping,
		Priority:               channel.Priority,
		Config:                 channel.Config,
		SystemPrompt:           channel.SystemPrompt,
		RateLimit:              channel.RateLimit,
		TestingModel:           channel.TestingModel,
		ModelRatio:             channel.ModelRatio,
		CompletionRatio:        channel.CompletionRatio,
		CreatedAt:              channel.CreatedAt,
		UpdatedAt:              channel.UpdatedAt,
		InferenceProfileArnMap: channel.InferenceProfileArnMap,
	}
}

// ChannelsToResponses maps a slice of channels to their external DTOs,
// pre-allocating the result.
//
// Parameters:
//   - channels: rows to convert; nil elements map to the zero shape.
//
// Return values:
//   - []dto.ChannelResponse: one entry per input row.
func ChannelsToResponses(channels []*Channel) []dto.ChannelResponse {
	out := make([]dto.ChannelResponse, 0, len(channels))
	for _, c := range channels {
		out = append(out, c.ToResponse())
	}
	return out
}
