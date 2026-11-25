package model

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/relay/channeltype"
)

// LoadConfig decodes stored channel settings and applies provider-specific URL
// security policy, including when Config is empty and only BaseURL is present.
func (channel *Channel) LoadConfig() (ChannelConfig, error) {
	var cfg ChannelConfig
	if channel.Config != "" {
		if err := json.Unmarshal([]byte(channel.Config), &cfg); err != nil {
			return cfg, identity.Tag(
				errors.Wrapf(err, "unmarshal channel %d config", channel.Id),
				channel.Ref())
		}
	}
	if channel.Type == channeltype.Jina {
		if err := channeltype.ValidateJinaURLs(channel.GetBaseURL(), cfg.EndpointURLs); err != nil {
			return cfg, identity.Tag(errors.Wrap(err, "validate Jina channel configuration"), channel.Ref())
		}
	}
	return cfg, nil
}

// storeConfig persists the provided ChannelConfig into the serialized Config
// column, clearing the field when the configuration is effectively empty.
func (channel *Channel) storeConfig(cfg ChannelConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "marshal channel %d config", channel.Id),
			channel.Ref())
	}
	if len(data) == 0 || string(data) == "{}" {
		channel.Config = ""
		return nil
	}
	channel.Config = string(data)
	return nil
}
