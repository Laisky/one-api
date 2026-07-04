package model

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// MarshalJSON emits the external channel contract without the internal integer identifier.
// Parameters: none.
//
// Return values:
//   - []byte: encoded JSON containing the channel UUID and channel attributes.
//   - error: wrapped marshal failure, if encoding fails.
func (channel Channel) MarshalJSON() ([]byte, error) {
	type channelJSON struct {
		UUID                   string  `json:"uuid"`
		Type                   int     `json:"type"`
		Key                    string  `json:"key"`
		Status                 int     `json:"status"`
		Name                   string  `json:"name"`
		Weight                 *uint   `json:"weight"`
		CreatedTime            int64   `json:"created_time"`
		TestTime               int64   `json:"test_time"`
		ResponseTime           int     `json:"response_time"`
		BaseURL                *string `json:"base_url"`
		Other                  *string `json:"other"`
		Balance                float64 `json:"balance"`
		BalanceUpdatedTime     int64   `json:"balance_updated_time"`
		Models                 string  `json:"models"`
		HiddenModels           *string `json:"hidden_models"`
		ModelConfigs           *string `json:"model_configs"`
		Group                  string  `json:"group"`
		UsedQuota              int64   `json:"used_quota"`
		ModelMapping           *string `json:"model_mapping"`
		Priority               *int64  `json:"priority"`
		Config                 string  `json:"config"`
		SystemPrompt           *string `json:"system_prompt"`
		RateLimit              *int    `json:"ratelimit"`
		TestingModel           *string `json:"testing_model"`
		ModelRatio             *string `json:"model_ratio"`
		CompletionRatio        *string `json:"completion_ratio"`
		CreatedAt              int64   `json:"created_at"`
		UpdatedAt              int64   `json:"updated_at"`
		InferenceProfileArnMap *string `json:"inference_profile_arn_map"`
	}

	payload, err := json.Marshal(channelJSON{
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
	})
	if err != nil {
		return nil, errors.Wrap(err, "marshal channel")
	}

	return payload, nil
}
