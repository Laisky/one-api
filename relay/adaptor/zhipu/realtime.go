package zhipu

import (
	"net/http"
	"net/url"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// realtimeUpstreamURL returns the GLM-Realtime WebSocket endpoint. Verified
// against the live API (2026-08-17): the model is selected via the `model`
// query parameter at connect time; sending `session.model` inside a
// session.update event makes the upstream close the connection.
func realtimeUpstreamURL(modelName string) string {
	u := "wss://open.bigmodel.cn/api/paas/v4/realtime"
	if modelName == "" {
		return u
	}
	return u + "?model=" + url.QueryEscape(modelName)
}

// RealtimeHandler proxies a WebSocket session to the Zhipu GLM-Realtime
// endpoint (wss://open.bigmodel.cn/api/paas/v4/realtime). The frame protocol
// mirrors OpenAI Realtime (session.update / input_audio_buffer.append /
// response.create / response.done), so frames are relayed transparently while
// token usage is parsed from upstream `response.done` events for billing.
//
// Parameters: c is the gin context and meta carries the channel identity and
// API key. Returns: a business error on handshake/connect failures and the
// parsed usage after the session closes.
func RealtimeHandler(c *gin.Context, meta *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage) {
	return realtimeHandlerAt(c, meta, realtimeUpstreamURL(meta.ActualModelName))
}

// realtimeHandlerAt runs the GLM-Realtime proxy against endpoint. RealtimeHandler
// always passes the fixed GLM-Realtime URL; tests pass a loopback provider so the
// handshake contract is exercised without contacting Zhipu.
//
// Parameters:
//   - c: the gin context of the authenticated realtime request.
//   - meta: the channel identity and API key.
//   - endpoint: the upstream WebSocket URL, including the model query.
//
// Returns:
//   - *model.ErrorWithStatusCode: a business error when the session could not
//     start (invalid mode, downstream upgrade or upstream handshake failure).
//   - *model.Usage: the parsed usage after a started session closes.
func realtimeHandlerAt(c *gin.Context, meta *meta.Meta, endpoint string) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
	if meta.Mode != relaymode.Realtime {
		return &model.ErrorWithStatusCode{
			Error:      model.Error{Message: "invalid mode for realtime handler", Type: model.ErrorTypeOneAPI, Code: "invalid_mode", RawError: errors.New("invalid mode for realtime handler")},
			StatusCode: http.StatusBadRequest,
		}, nil
	}

	// Upgrade the downstream connection, echoing client subprotocols (except
	// auth-carrying ones) so browser clients accept the handshake.
	upgrader := websocket.Upgrader{
		CheckOrigin:      func(r *http.Request) bool { return true },
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     openai.NegotiateRealtimeSubprotocols(c.Request),
	}
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return &model.ErrorWithStatusCode{
			Error:      model.Error{Message: "websocket upgrade failed: " + err.Error(), Type: model.ErrorTypeOneAPI, Code: "ws_upgrade_failed", RawError: err},
			StatusCode: http.StatusBadRequest,
		}, nil
	}
	defer func() { _ = clientConn.Close() }()

	// Dial the GLM-Realtime upstream with the channel API key. Zhipu accepts
	// the raw {id}.{secret} key as a Bearer token.
	requestHeader := http.Header{}
	if sp := c.GetHeader("Sec-WebSocket-Protocol"); sp != "" {
		requestHeader.Set("Sec-WebSocket-Protocol", sp)
	}
	requestHeader.Set("Authorization", "Bearer "+meta.APIKey)

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	upstreamConn, handshake, derr := dialer.Dial(endpoint, requestHeader)
	if derr != nil {
		if werr := clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream connect failed")); werr != nil {
			lg.Debug("write zhipu realtime upstream-connect close frame failed", zap.Error(werr))
		}
		// The client is already upgraded, so it only sees the close frame above;
		// the returned error serves relay accounting, which logs it with the
		// channel context, so it is returned rather than also logged here. A
		// provider that refused the handshake with 400..599 (for example 401,
		// 429 or 503) is attributed to the channel with its own status; a dial or
		// transport failure without a provider status stays one-api's own 502.
		// The message never includes the provider's handshake body: gorilla
		// reports every refused upgrade as "websocket: bad handshake".
		status, fromUpstream := openai.WebSocketHandshakeFailureStatus(lg, handshake)
		errorType := model.ErrorTypeOneAPI
		if fromUpstream {
			errorType = model.ErrorTypeUpstream
		}
		return &model.ErrorWithStatusCode{
			Error: model.Error{Message: "upstream realtime connect failed: " + derr.Error(), Type: errorType,
				Code: "upstream_connect_failed", RawError: errors.Wrap(derr, "dial zhipu realtime websocket"), FromUpstream: fromUpstream},
			StatusCode: status,
		}, nil
	}
	defer func() { _ = upstreamConn.Close() }()

	// GLM-Realtime selects the model via session.update, so the OpenAI
	// session-model guard is disabled.
	usage := openai.RealtimeBidirectionalPump(clientConn, upstreamConn, false, lg, meta.ActualModelName, meta.OriginModelName)
	return nil, usage
}
