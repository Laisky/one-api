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
	upstreamConn, _, derr := dialer.Dial(realtimeUpstreamURL(meta.ActualModelName), requestHeader)
	if derr != nil {
		_ = clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream connect failed"))
		lg.Error("zhipu realtime upstream connect failed", zap.Error(derr))
		return &model.ErrorWithStatusCode{
			Error:      model.Error{Message: "upstream realtime connect failed: " + derr.Error(), Type: model.ErrorTypeUpstream, Code: "upstream_connect_failed", RawError: derr},
			StatusCode: http.StatusBadGateway,
		}, nil
	}
	defer func() { _ = upstreamConn.Close() }()

	// GLM-Realtime selects the model via session.update, so the OpenAI
	// session-model guard is disabled.
	usage := openai.RealtimeBidirectionalPump(clientConn, upstreamConn, false, lg)
	return nil, usage
}
