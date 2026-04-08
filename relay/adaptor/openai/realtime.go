package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	rmeta "github.com/songquanpeng/one-api/relay/meta"
	rmodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// RealtimeSessionsHandler proxies a POST request to the upstream OpenAI
// Realtime Sessions endpoint (/v1/realtime/sessions) which creates ephemeral
// tokens for WebRTC browser clients.
func RealtimeSessionsHandler(c *gin.Context, meta *rmeta.Meta) (*rmodel.ErrorWithStatusCode, error) {
	// Read the incoming request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "failed to read request body: " + err.Error(), Type: rmodel.ErrorTypeOneAPI, Code: "read_body_failed", RawError: err},
			StatusCode: http.StatusBadRequest,
		}, errors.Wrap(err, "read request body")
	}

	// Build upstream URL
	base := meta.BaseURL
	if base == "" {
		base = "https://api.openai.com"
	}
	upstreamURL := strings.TrimRight(base, "/") + "/v1/realtime/sessions"

	// Create upstream request
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "failed to create upstream request: " + err.Error(), Type: rmodel.ErrorTypeInternal, Code: "create_request_failed", RawError: err},
			StatusCode: http.StatusInternalServerError,
		}, errors.Wrap(err, "create upstream request")
	}
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", "realtime=v1")

	// Send request to upstream
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "upstream realtime sessions request failed: " + err.Error(), Type: rmodel.ErrorTypeUpstream, Code: "upstream_request_failed", RawError: err},
			StatusCode: http.StatusBadGateway,
		}, errors.Wrap(err, "upstream request")
	}
	defer resp.Body.Close()

	// Copy upstream response headers
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}

	// Copy status code and body back to client
	c.Writer.WriteHeader(resp.StatusCode)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "failed to read upstream response: " + err.Error(), Type: rmodel.ErrorTypeUpstream, Code: "read_upstream_failed", RawError: err},
			StatusCode: http.StatusBadGateway,
		}, errors.Wrap(err, "read upstream response")
	}
	_, _ = c.Writer.Write(respBody)

	// If upstream returned a non-2xx status, surface it as a business error
	if resp.StatusCode >= 400 {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: string(respBody), Type: rmodel.ErrorTypeUpstream, Code: resp.StatusCode},
			StatusCode: resp.StatusCode,
		}, nil
	}

	return nil, nil
}

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

	// Upgrade downstream connection.
	// For browser clients using subprotocol-based auth, negotiate the "realtime"
	// subprotocol so the browser accepts the connection. Filter out auth/beta
	// subprotocols that should not be echoed back.
	upgrader := websocket.Upgrader{
		CheckOrigin:      func(r *http.Request) bool { return true },
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     negotiateRealtimeSubprotocols(c.Request),
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

// negotiateRealtimeSubprotocols extracts subprotocols from the client request
// that should be echoed back during the WebSocket handshake. Auth-related
// subprotocols (openai-insecure-api-key.*) are filtered out so they are not
// echoed to the client.
func negotiateRealtimeSubprotocols(r *http.Request) []string {
	sp := r.Header.Get("Sec-WebSocket-Protocol")
	if sp == "" {
		return nil
	}
	var result []string
	for _, proto := range strings.Split(sp, ",") {
		proto = strings.TrimSpace(proto)
		// Don't echo back the auth token subprotocol
		if strings.HasPrefix(proto, "openai-insecure-api-key.") {
			continue
		}
		if proto != "" {
			result = append(result, proto)
		}
	}
	return result
}

// maybeParseRealtimeUsage attempts to extract token usage from response.done-like events.
// It deduplicates by response ID to avoid double-counting.
// It also extracts input_token_details and output_token_details for audio/text
// breakdown, which is critical for correct realtime billing (audio tokens are
// significantly more expensive than text tokens).
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

	// input_tokens / output_tokens totals
	if v, ok := usageObj["input_tokens"].(float64); ok {
		u.PromptTokens += int(v)
	}
	if v, ok := usageObj["output_tokens"].(float64); ok {
		u.CompletionTokens += int(v)
	}
	if v, ok := usageObj["total_tokens"].(float64); ok {
		u.TotalTokens += int(v)
	}

	// Parse input_token_details for audio/text/cached breakdown
	if details, ok := usageObj["input_token_details"].(map[string]any); ok {
		if u.PromptTokensDetails == nil {
			u.PromptTokensDetails = &rmodel.UsagePromptTokensDetails{}
		}
		if v, ok := details["cached_tokens"].(float64); ok {
			u.PromptTokensDetails.CachedTokens += int(v)
		}
		if v, ok := details["audio_tokens"].(float64); ok {
			u.PromptTokensDetails.AudioTokens += int(v)
		}
		if v, ok := details["text_tokens"].(float64); ok {
			u.PromptTokensDetails.TextTokens += int(v)
		}
	}

	// Parse output_token_details for audio/text breakdown
	if details, ok := usageObj["output_token_details"].(map[string]any); ok {
		if u.CompletionTokensDetails == nil {
			u.CompletionTokensDetails = &rmodel.UsageCompletionTokensDetails{}
		}
		if v, ok := details["audio_tokens"].(float64); ok {
			u.CompletionTokensDetails.AudioTokens += int(v)
		}
		if v, ok := details["text_tokens"].(float64); ok {
			u.CompletionTokensDetails.TextTokens += int(v)
		}
	}
}
