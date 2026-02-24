package controller

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// maybeHandleResponseAPIWebSocket handles websocket upgrades for /v1/responses.
//
// Parameters:
//   - c: request context.
//   - meta: relay metadata resolved from middleware.
//
// Returns:
//   - bool: true when this request was a websocket upgrade and has been handled.
//   - *relaymodel.ErrorWithStatusCode: business error when websocket handling fails.
func maybeHandleResponseAPIWebSocket(c *gin.Context, meta *metalib.Meta) (bool, *relaymodel.ErrorWithStatusCode) {
	if !websocket.IsWebSocketUpgrade(c.Request) {
		return false, nil
	}

	if meta == nil {
		return true, openai.ErrorWrapper(errors.New("missing relay meta"), "invalid_meta", http.StatusBadRequest)
	}

	if meta.ChannelType != channeltype.OpenAI {
		return true, openai.ErrorWrapper(
			errors.New("response websocket is only supported for OpenAI channels"),
			"response_websocket_only_supported_for_openai_channel",
			http.StatusBadRequest,
		)
	}

	if !supportsNativeResponseAPI(meta) {
		return true, openai.ErrorWrapper(
			errors.New("response websocket is not supported for this channel"),
			"response_websocket_not_supported_for_channel",
			http.StatusBadRequest,
		)
	}

	if bizErr, _ := openai.ResponseAPIWebSocketHandler(c, meta); bizErr != nil {
		return true, bizErr
	}

	return true, nil
}
