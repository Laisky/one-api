package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	rmeta "github.com/songquanpeng/one-api/relay/meta"
	rmodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// RealtimeHandler proxies a WebSocket session to the upstream OpenAI Realtime endpoint.
// It preserves text/binary frames and mirrors the `Sec-WebSocket-Protocol` when present.
func RealtimeHandler(c *gin.Context, meta *rmeta.Meta) (*rmodel.ErrorWithStatusCode, *rmodel.Usage) {
	lg := gmw.GetLogger(c)
	if meta.Mode != relaymode.Realtime {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "invalid mode for realtime handler", Type: rmodel.ErrorTypeOneAPI, Code: "invalid_mode", RawError: errors.New("invalid mode for realtime handler")},
			StatusCode: http.StatusBadRequest,
		}, nil
	}

	// Upgrade downstream connection
	upgrader := websocket.Upgrader{
		CheckOrigin:      func(r *http.Request) bool { return true },
		HandshakeTimeout: 10 * time.Second,
	}

	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "websocket upgrade failed: " + err.Error(), Type: rmodel.ErrorTypeOneAPI, Code: "ws_upgrade_failed", RawError: err},
			StatusCode: http.StatusBadRequest,
		}, nil
	}
	// Ensure close on exit
	defer func() { _ = clientConn.Close() }()

	// Build upstream URL
	base := meta.BaseURL
	if base == "" {
		base = "https://api.openai.com" // fallback
	}
	// Preserve query but ensure model uses mapped ActualModelName
	rawQuery := c.Request.URL.RawQuery
	u, _ := url.Parse(base)

	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1) // http->ws, https->wss
	switch u.Scheme {
	case "", "http":
		u.Scheme = "wss"
	case "https":
		u.Scheme = "wss"
	}

	u.Path = "/v1/realtime"
	// Override model query with mapped model if provided
	q, _ := url.ParseQuery(rawQuery)
	if meta.ActualModelName != "" {
		q.Set("model", meta.ActualModelName)
	}
	u.RawQuery = q.Encode()

	// Prepare headers and subprotocols
	requestHeader := http.Header{}
	if sp := c.GetHeader("Sec-WebSocket-Protocol"); sp != "" {
		requestHeader.Set("Sec-WebSocket-Protocol", sp)
	}
	if beta := c.GetHeader("OpenAI-Beta"); beta != "" {
		requestHeader.Set("OpenAI-Beta", beta)
	} else {
		// Default beta header required by OpenAI Realtime during beta period
		requestHeader.Set("OpenAI-Beta", "realtime=v1")
	}
	requestHeader.Set("Authorization", "Bearer "+meta.APIKey)

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	upstreamConn, _, derr := dialer.Dial(u.String(), requestHeader)
	if derr != nil {
		_ = clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream connect failed"))
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "upstream realtime connect failed: " + derr.Error(), Type: rmodel.ErrorTypeOneAPI, Code: "upstream_connect_failed", RawError: derr},
			StatusCode: http.StatusBadGateway,
		}, nil
	}
	defer func() { _ = upstreamConn.Close() }()

	// Bi-directional pump
	errc := make(chan error, 2)
	usage := &rmodel.Usage{}
	countedResponseIDs := map[string]struct{}{}
	go func() { errc <- copyWSUpstreamToClient(upstreamConn, clientConn, usage, countedResponseIDs) }()
	go func() { errc <- copyWS(clientConn, upstreamConn) }()

	// Wait for one direction to finish, then close both connections
	// to unblock the other goroutine.
	if e := <-errc; e != nil {
		lg.Debug("realtime ws first direction closed", zap.Error(e))
	}
	_ = clientConn.Close()
	_ = upstreamConn.Close()

	// Drain the second goroutine to avoid data race on usage.
	if e := <-errc; e != nil {
		lg.Debug("realtime ws second direction closed", zap.Error(e))
	}

	// Compute total tokens if we have parts
	if usage != nil && usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	// Record a zero-quota realtime session log for observability.
	// Realtime currently does not perform quota billing, but we still want latency & usage visibility.
	go func() {
		ctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), time.Minute)
		defer cancel()

		requestId := c.GetString(ctxkey.RequestId)
		traceId := tracing.GetTraceID(c)
		model.RecordConsumeLog(ctx, &model.Log{
			UserId:    meta.UserId,
			ChannelId: meta.ChannelId,
			PromptTokens: func() int {
				if usage != nil {
					return usage.PromptTokens
				}
				return 0
			}(),
			CompletionTokens: func() int {
				if usage != nil {
					return usage.CompletionTokens
				}
				return 0
			}(),
			ModelName:   meta.ActualModelName,
			TokenName:   meta.TokenName,
			Quota:       0,
			Content:     "realtime session, no quota consumption",
			IsStream:    true,
			ElapsedTime: helper.CalcElapsedTime(meta.StartTime),
			RequestId:   requestId,
			TraceId:     traceId,
		})
	}()

	return nil, usage
}

func copyWS(src, dst *websocket.Conn) error {
	for {
		mt, msg, err := src.ReadMessage()
		if err != nil {
			// Propagate close frame to the other side so it gets a clean shutdown.
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				_ = dst.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(closeErr.Code, closeErr.Text),
					time.Now().Add(time.Second),
				)
				return nil
			}
			return errors.WithStack(err)
		}
		// Mirror frame type
		if werr := dst.WriteMessage(mt, msg); werr != nil {
			return errors.WithStack(werr)
		}
	}
}

// copyWSUpstreamToClient forwards frames and tries best-effort to parse usage from upstream JSON text messages.
func copyWSUpstreamToClient(src, dst *websocket.Conn, usage *rmodel.Usage, countedResponseIDs map[string]struct{}) error {
	for {
		mt, msg, err := src.ReadMessage()
		if err != nil {
			// Propagate close frame to the other side.
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				_ = dst.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(closeErr.Code, closeErr.Text),
					time.Now().Add(time.Second),
				)
				return nil
			}
			return errors.WithStack(err)
		}
		if mt == websocket.TextMessage {
			maybeParseRealtimeUsage(msg, usage, countedResponseIDs)
		}
		if werr := dst.WriteMessage(mt, msg); werr != nil {
			return errors.WithStack(werr)
		}
	}
}

// maybeParseRealtimeUsage attempts to extract token usage from response.done-like events.
// It deduplicates by response ID to avoid double-counting.
func maybeParseRealtimeUsage(msg []byte, u *rmodel.Usage, countedResponseIDs map[string]struct{}) {
	// Avoid heavy processing if no accumulator
	if u == nil || len(msg) == 0 {
		return
	}
	// Very permissive JSON parsing into generic map
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		return
	}
	// Expect events with type and nested response.usage
	resp, _ := m["response"].(map[string]any)
	if resp == nil {
		return
	}
	usageObj, _ := resp["usage"].(map[string]any)
	if usageObj == nil {
		return
	}

	// Deduplicate by response ID to prevent double-counting
	responseID, _ := resp["id"].(string)
	if responseID != "" && countedResponseIDs != nil {
		if _, exists := countedResponseIDs[responseID]; exists {
			return
		}
		countedResponseIDs[responseID] = struct{}{}
	}

	// input_tokens / output_tokens are preferred
	if v, ok := usageObj["input_tokens"].(float64); ok {
		u.PromptTokens += int(v)
	}
	if v, ok := usageObj["output_tokens"].(float64); ok {
		u.CompletionTokens += int(v)
	}
	if v, ok := usageObj["total_tokens"].(float64); ok {
		// If total provided, prefer exact
		u.TotalTokens += int(v)
	}
}
