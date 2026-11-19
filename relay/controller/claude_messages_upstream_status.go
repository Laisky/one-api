package controller

import (
	"net/http"

	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// claudeUpstreamFailed reports whether the provider answered a Claude Messages
// attempt with an HTTP error status. It mirrors isErrorHappened's status rule
// for the Claude controller: 200 is success, and Replicate acknowledges a
// prediction with 201 Created, which its adaptor then polls to completion.
// Parameters: meta is the request meta and resp the provider response, nil
// for SDK transports that return no HTTP response. Returns: true only for a
// non-nil response whose status is an error for that provider.
func claudeUpstreamFailed(meta *metalib.Meta, resp *http.Response) bool {
	if resp == nil || resp.StatusCode == http.StatusOK {
		return false
	}
	return resp.StatusCode != http.StatusCreated || meta == nil || meta.ChannelType != channeltype.Replicate
}
