package model

import (
	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/channeltype"
)

// validateJinaChannelConfiguration applies Jina's config-load validation before
// a channel transaction commits. Other providers preserve their existing policy.
func validateJinaChannelConfiguration(channel *Channel) error {
	if channel.Type != channeltype.Jina {
		return nil
	}
	if _, err := channel.LoadConfig(); err != nil {
		return errors.Wrap(err, "validate credential-bearing Jina URLs")
	}
	return nil
}
