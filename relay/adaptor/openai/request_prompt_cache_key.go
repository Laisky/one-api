package openai

import (
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// ScopePromptCacheKey applies the prompt_cache_key forwarding policy for the
// destination described by metaInfo, modifying key in place.
//
// Only native OpenAI channels (channeltype.OpenAI) receive the key, and only as
// model.NamespacePromptCacheKey of the gateway user and the client value: this
// gateway shares one upstream organisation across tenants, so a raw key would
// let two tenants share a cache partition. Every other channel type has the key
// withheld, because OpenAI-compatible hosts may reject unknown fields. A request
// without an authenticated user (UserId <= 0) cannot be namespaced and is
// withheld too.
//
// Parameters: lg is the request-scoped logger (may be nil), metaInfo is the
// destination, key is the request's key and route names the protocol path for
// diagnostics. It returns true when a scoped key will be forwarded.
func ScopePromptCacheKey(lg glog.Logger, metaInfo *meta.Meta, key *model.PromptCacheKey, route string) bool {
	if key == nil {
		return false
	}
	channelType := 0
	userID := 0
	if metaInfo != nil {
		channelType = metaInfo.ChannelType
		userID = metaInfo.UserId
	}
	if key.Invalid() {
		key.Withhold()
		if lg != nil {
			lg.Debug("dropped malformed prompt_cache_key; serving the request without cache routing",
				zap.String("route", route))
		}
		return false
	}
	if channelType != channeltype.OpenAI || userID <= 0 {
		key.Withhold()
		if key.Present() && lg != nil {
			lg.Debug("withheld prompt_cache_key from destination",
				zap.String("route", route),
				zap.Int("channel_type", channelType),
				zap.Bool("authenticated_user", userID > 0),
			)
		}
		return false
	}
	if !key.ScopeToUser(userID) {
		return false
	}
	if lg != nil {
		// Never log the client or forwarded key: either value identifies a
		// tenant's cache partition.
		lg.Debug("forwarding tenant-scoped prompt_cache_key to native OpenAI",
			zap.String("route", route),
		)
	}
	return true
}
