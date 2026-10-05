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

	rmeta "github.com/Laisky/one-api/relay/meta"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

type responseAPIWebSocketEvent struct {
	Type     string                    `json:"type"`
	Response *responseAPIEventResponse `json:"response,omitempty"`
	Error    map[string]any            `json:"error,omitempty"`
	Status   int                       `json:"status,omitempty"`
}

type responseAPIEventResponse struct {
	ID     string                 `json:"id,omitempty"`
	Status string                 `json:"status,omitempty"`
	Model  string                 `json:"model,omitempty"`
	Usage  *responseAPIEventUsage `json:"usage,omitempty"`
}

type responseAPIEventUsage struct {
	InputTokens        int `json:"input_tokens,omitempty"`
	OutputTokens       int `json:"output_tokens,omitempty"`
	TotalTokens        int `json:"total_tokens,omitempty"`
	CacheWriteTokens   int `json:"cache_write_tokens,omitempty"`
	CacheWrite5mTokens int `json:"cache_write_5m_tokens,omitempty"`
	CacheWrite1hTokens int `json:"cache_write_1h_tokens,omitempty"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens,omitempty"`
	} `json:"input_tokens_details,omitempty"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	} `json:"output_tokens_details,omitempty"`
}

// ResponseAPIWebSocketHandler proxies a user websocket connection from /v1/responses
// to the upstream OpenAI /v1/responses websocket endpoint.
//
// Parameters:
//   - c: request context carrying client websocket upgrade request.
//   - meta: relay metadata with upstream credentials and base URL.
//
// Returns:
//   - *rmodel.ErrorWithStatusCode: business error when proxying fails.
//   - *rmodel.Usage: best-effort aggregated usage parsed from upstream events.
//   - []*ResponseAPIResponse: store!=false completed response objects observed on
//     the socket (proposal ST-011). The native passthrough leaves connection-local
//     store=false state to the upstream; the caller commits these observed
//     store=true responses to the gateway store so their IDs are retrievable over
//     HTTP afterwards. The slice is populated only after both proxy directions
//     drain, so it is safe to read without further synchronization.
func ResponseAPIWebSocketHandler(c *gin.Context, meta *rmeta.Meta) (*rmodel.ErrorWithStatusCode, *rmodel.Usage, []*ResponseAPIResponse) {
	if meta == nil || meta.Mode != relaymode.ResponseAPI {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "invalid mode for response websocket handler", Type: rmodel.ErrorTypeOneAPI, Code: "invalid_mode", RawError: errors.New("invalid mode for response websocket handler")},
			StatusCode: http.StatusBadRequest,
		}, nil, nil
	}

	upgrader := websocket.Upgrader{
		CheckOrigin:      func(r *http.Request) bool { return true },
		HandshakeTimeout: 10 * time.Second,
	}

	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "websocket upgrade failed: " + err.Error(), Type: rmodel.ErrorTypeOneAPI, Code: "ws_upgrade_failed", RawError: err},
			StatusCode: http.StatusBadRequest,
		}, nil, nil
	}
	defer func() { _ = clientConn.Close() }()

	fullRequestURL, err := resolveResponseAPIWebSocketUpstreamURL(c, meta)
	if err != nil {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "resolve upstream url failed", Type: rmodel.ErrorTypeInternal, Code: "upstream_url_resolve_failed", RawError: err},
			StatusCode: http.StatusBadGateway,
		}, nil, nil
	}

	requestHeader := http.Header{}
	// Drop the client's "openai-insecure-api-key.*" subprotocol before dialing.
	// Sending it together with the channel's Authorization header makes OpenAI
	// reject the handshake with "You must only send one of protocol api key and
	// Authorization header".
	if sp := NegotiateRealtimeSubprotocols(c.Request); len(sp) > 0 {
		requestHeader.Set("Sec-WebSocket-Protocol", strings.Join(sp, ", "))
	}
	requestHeader.Set("Authorization", "Bearer "+meta.APIKey)
	requestHeader.Set("OpenAI-Beta", "responses-api=v1")

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	upstreamConn, _, dialErr := dialer.Dial(fullRequestURL, requestHeader)
	if dialErr != nil {
		_ = clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream connect failed"))
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "upstream response websocket connect failed: " + dialErr.Error(), Type: rmodel.ErrorTypeUpstream, Code: "upstream_connect_failed", RawError: dialErr},
			StatusCode: http.StatusBadGateway,
		}, nil, nil
	}
	defer func() { _ = upstreamConn.Close() }()

	usage := &rmodel.Usage{}
	// stored collects the store!=false completed response objects observed on the
	// upstream->client leg. It is written only by that single goroutine and read
	// only after both legs drain below, so no further synchronization is needed.
	ownership := &responseWSSessionOwnership{}
	stored := &responseAPIWSStoreCollector{ownership: ownership}
	errc := make(chan error, 2)
	requestContext := c.Request.Context()
	go func() {
		errc <- copyResponseAPIClientToUpstream(requestContext, clientConn, upstreamConn, meta, ownership)
	}()
	receipts := &responseAPIWSUsageCollector{usage: usage}
	go func() { errc <- copyResponseAPIWSUpstreamToClient(upstreamConn, clientConn, receipts, stored) }()

	// Wait for one direction to finish, then close both connections
	// to unblock the other goroutine.
	firstErr := <-errc
	lg := gmw.GetLogger(c)
	if closeErr := clientConn.Close(); closeErr != nil {
		lg.Debug("close response client socket", zap.Error(closeErr))
	}
	if closeErr := upstreamConn.Close(); closeErr != nil {
		lg.Debug("close response upstream socket", zap.Error(closeErr))
	}
	secondErr := <-errc
	// Both legs have drained, so the collector and dispatch counter are stable.
	receipts.finish(ownership.creates.Load())
	// An admission failure before any execution must release the handshake hold.
	// If an earlier frame was dispatched, preserve its billing reconciliation.
	if ownership.denied.Load() && !ownership.dispatched.Load() {
		return ErrorWrapper(errors.New("response websocket request was rejected before execution"), "response_websocket_request_rejected", http.StatusBadRequest), nil, nil
	}
	if firstErr != nil {
		lg.Debug("response websocket first direction closed", zap.Error(firstErr))
	}
	if secondErr != nil {
		lg.Debug("response websocket second direction closed", zap.Error(secondErr))
	}

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return nil, usage, stored.responses
}

// responseAPIWSStoreCollector accumulates store!=false completed response objects
// observed on the upstream->client leg of a native Responses websocket. store=false
// responses are connection-local upstream state and are intentionally excluded
// (proposal Section 5.9, SEC06).
type responseAPIWSStoreCollector struct {
	ownership *responseWSSessionOwnership
	responses []*ResponseAPIResponse
	seen      map[string]struct{}
}

// collect records one completed response object if it is retrievable (store!=false)
// and not already captured. It parses only the terminal response.completed event;
// all other frames are ignored.
func (c *responseAPIWSStoreCollector) collect(msg []byte) {
	if len(msg) == 0 {
		return
	}
	var probe struct {
		Type     string          `json:"type"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(msg, &probe); err != nil {
		return
	}
	if probe.Type != "response.completed" || len(probe.Response) == 0 {
		return
	}
	var resp ResponseAPIResponse
	if err := json.Unmarshal(probe.Response, &resp); err != nil || resp.Id == "" {
		return
	}
	if !isResponseAPIWSTerminalReceipt(probe.Type, resp.Status) {
		return
	}
	if c.ownership != nil {
		c.ownership.observe(resp.Id)
	}
	// store defaults to true; only an explicit store=false is connection-local.
	if resp.Store != nil && !*resp.Store {
		return
	}
	if c.seen == nil {
		c.seen = map[string]struct{}{}
	}
	if _, dup := c.seen[resp.Id]; dup {
		return
	}
	c.seen[resp.Id] = struct{}{}
	c.responses = append(c.responses, &resp)
}

// resolveResponseAPIWebSocketUpstreamURL builds the upstream ws(s) URL for the
// responses endpoint while preserving query parameters.
func resolveResponseAPIWebSocketUpstreamURL(c *gin.Context, meta *rmeta.Meta) (string, error) {
	base := strings.TrimSpace(meta.BaseURL)
	if base == "" {
		base = "https://api.openai.com"
	}

	u, err := url.Parse(base)
	if err != nil {
		return "", errors.Wrap(err, "parse upstream base url")
	}

	switch u.Scheme {
	case "https", "wss", "":
		u.Scheme = "wss"
	case "http", "ws":
		u.Scheme = "ws"
	default:
		return "", errors.Errorf("unsupported upstream url scheme: %s", u.Scheme)
	}

	u.Path = "/v1/responses"
	u.RawQuery = c.Request.URL.RawQuery
	return u.String(), nil
}

// copyResponseAPIClientToUpstream forwards client frames to the upstream
// connection while enforcing model pinning on every `response.create` event.
//
// On the first frame that attempts to switch the model away from the
// handshake-bound model, the function:
//   - emits a `model_switch_denied` error event back to the client
//   - returns ErrModelSwitchDenied (wrapped) so the caller closes both legs
//
// Ownership is checked for both text and binary JSON frames before dispatch.
//
// Parameters:
//   - src: client WebSocket connection (reader).
//   - dst: upstream WebSocket connection (writer).
//   - ctx: request cancellation context.
//   - meta: authenticated owner and handshake-bound provider/model.
//   - ownership: response IDs confirmed on this connection.
//
// Returns:
//   - error: nil on clean close; ErrModelSwitchDenied (wrapped) on rejected
//     model switch; other errors propagate underlying I/O failures.
func copyResponseAPIClientToUpstream(ctx context.Context, src, dst *websocket.Conn, meta *rmeta.Meta, ownership *responseWSSessionOwnership) error {
	boundOriginModel, boundActualModel := meta.OriginModelName, meta.ActualModelName
	for {
		mt, msg, err := src.ReadMessage()
		if err != nil {
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

		outbound, authorizationErr := ownership.authorize(ctx, meta, msg)
		if authorizationErr != nil {
			ownership.denied.Store(true)
			// Control writes are safe alongside the upstream-to-client writer.
			reason := responseWSAuthorizationCloseReason(authorizationErr)
			if closeErr := src.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, reason), time.Now().Add(time.Second)); closeErr != nil {
				return errors.Wrap(closeErr, "close unauthorized response websocket")
			}
			return errors.WithStack(authorizationErr)
		}
		if (mt == websocket.TextMessage || mt == websocket.BinaryMessage) && boundActualModel != "" {
			rewritten, guardErr := enforceResponseCreateModel(outbound, boundOriginModel, boundActualModel)
			if guardErr != nil {
				// Reject: notify the client with an error event and close the
				// upstream side so billing reconciliation runs without any
				// upstream charges accruing for the denied model.
				errEvent := buildModelSwitchErrorEvent(guardErr.Error())
				_ = src.WriteMessage(websocket.TextMessage, errEvent)
				_ = src.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "model_switch_denied"),
					time.Now().Add(time.Second),
				)
				return errors.WithStack(guardErr)
			}
			outbound = rewritten
		}

		if werr := dst.WriteMessage(mt, outbound); werr != nil {
			return errors.WithStack(werr)
		}
		ownership.dispatched.Store(true)
		if isResponseCreateFrame(outbound) {
			ownership.creates.Add(1)
		}
	}
}

// copyResponseAPIWSUpstreamToClient forwards upstream frames to the client and
// feeds response events to receipts, which extracts usage from qualified
// terminal receipts; the caller finishes receipts after both legs drain. It
// also records store!=false completed response objects into stored so the
// caller can commit them to the gateway store (proposal ST-011). Collection
// happens after usage accounting and never alters the forwarded frame.
func copyResponseAPIWSUpstreamToClient(src, dst *websocket.Conn, receipts *responseAPIWSUsageCollector, stored *responseAPIWSStoreCollector) error {

	for {
		mt, msg, err := src.ReadMessage()
		if err != nil {
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
			receipts.collect(msg)
			if stored != nil {
				stored.collect(msg)
			}
		}

		if werr := dst.WriteMessage(mt, msg); werr != nil {
			return errors.WithStack(werr)
		}
	}
}

// accumulateResponseAPIUsage parses one websocket text event and updates usage once
// per response ID to avoid double counting qualified terminal receipts.
func accumulateResponseAPIUsage(msg []byte, usage *rmodel.Usage, countedResponseIDs map[string]struct{}) {
	if usage == nil || len(msg) == 0 {
		return
	}

	responseID, snapshot, ok := extractResponseAPIUsage(msg)
	if !ok || responseID == "" {
		return
	}

	if _, exists := countedResponseIDs[responseID]; exists {
		return
	}
	countedResponseIDs[responseID] = struct{}{}

	usage.PromptTokens += snapshot.PromptTokens
	usage.CompletionTokens += snapshot.CompletionTokens
	usage.TotalTokens += snapshot.TotalTokens
	usage.CacheWrite5mTokens += snapshot.CacheWrite5mTokens
	usage.CacheWrite1hTokens += snapshot.CacheWrite1hTokens

	if snapshot.PromptTokensDetails != nil {
		if usage.PromptTokensDetails == nil {
			usage.PromptTokensDetails = &rmodel.UsagePromptTokensDetails{}
		}
		usage.PromptTokensDetails.CachedTokens += snapshot.PromptTokensDetails.CachedTokens
	}

	if snapshot.CompletionTokensDetails != nil {
		if usage.CompletionTokensDetails == nil {
			usage.CompletionTokensDetails = &rmodel.UsageCompletionTokensDetails{}
		}
		usage.CompletionTokensDetails.ReasoningTokens += snapshot.CompletionTokensDetails.ReasoningTokens
	}
}

// extractResponseAPIUsage extracts one response usage snapshot from one websocket payload.
func extractResponseAPIUsage(msg []byte) (string, *rmodel.Usage, bool) {
	event := responseAPIWebSocketEvent{}
	if err := json.Unmarshal(msg, &event); err != nil {
		return "", nil, false
	}

	if event.Response == nil || event.Response.Usage == nil || event.Response.ID == "" ||
		!isResponseAPIWSTerminalReceipt(event.Type, event.Response.Status) {
		return "", nil, false
	}

	usage := &rmodel.Usage{
		PromptTokens:       event.Response.Usage.InputTokens,
		CompletionTokens:   event.Response.Usage.OutputTokens,
		TotalTokens:        event.Response.Usage.TotalTokens,
		CacheWrite5mTokens: event.Response.Usage.CacheWriteTokens + event.Response.Usage.CacheWrite5mTokens,
		CacheWrite1hTokens: event.Response.Usage.CacheWrite1hTokens,
	}

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	if event.Response.Usage.InputTokensDetails.CachedTokens > 0 {
		usage.PromptTokensDetails = &rmodel.UsagePromptTokensDetails{
			CachedTokens: event.Response.Usage.InputTokensDetails.CachedTokens,
		}
	}

	if event.Response.Usage.OutputTokensDetails.ReasoningTokens > 0 {
		usage.CompletionTokensDetails = &rmodel.UsageCompletionTokensDetails{
			ReasoningTokens: event.Response.Usage.OutputTokensDetails.ReasoningTokens,
		}
	}

	return event.Response.ID, usage, true
}
