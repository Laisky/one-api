package deepseekcompat

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
)

// UsesDeepSeekToolReceiptContract reports whether the planned dispatch target
// uses DeepSeek's undocumented paid-tool receipt contract. info is the current
// dispatch metadata. A full endpoint override replaces the channel/base target;
// the previously observed request URL is deliberately ignored after retries.
func UsesDeepSeekToolReceiptContract(info *meta.Meta) bool {
	if info == nil {
		return false
	}
	if endpoint := info.UpstreamEndpointURLOverride(); endpoint != "" {
		return HostUsesDeepSeekAPIContract(endpoint)
	}
	return UsesDeepSeekAPIContract(info)
}

// UsesDeepSeekToolReceiptContractFromContext identifies the active upstream contract
// without rebuilding metadata or inspecting credentials. Cached request metadata
// is authoritative; contexts without it use the existing channel/base URL facts.
func UsesDeepSeekToolReceiptContractFromContext(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if raw, ok := c.Get(ctxkey.Meta); ok {
		if info, ok := raw.(*meta.Meta); ok && info != nil {
			return UsesDeepSeekToolReceiptContract(info)
		}
	}
	return UsesDeepSeekToolReceiptContract(&meta.Meta{ChannelType: c.GetInt(ctxkey.Channel), BaseURL: c.GetString(ctxkey.BaseURL)})
}
