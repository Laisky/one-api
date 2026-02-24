package openai

import (
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

	rmeta "github.com/songquanpeng/one-api/relay/meta"
	rmodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type responseAPIWebSocketEvent struct {
	Type     string                    `json:"type"`
	Response *responseAPIEventResponse `json:"response,omitempty"`
	Error    map[string]any            `json:"error,omitempty"`
	Status   int                       `json:"status,omitempty"`
}

type responseAPIEventResponse struct {
	ID    string                 `json:"id,omitempty"`
	Model string                 `json:"model,omitempty"`
	Usage *responseAPIEventUsage `json:"usage,omitempty"`
}

type responseAPIEventUsage struct {
	InputTokens        int `json:"input_tokens,omitempty"`
	OutputTokens       int `json:"output_tokens,omitempty"`
	TotalTokens        int `json:"total_tokens,omitempty"`
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
func ResponseAPIWebSocketHandler(c *gin.Context, meta *rmeta.Meta) (*rmodel.ErrorWithStatusCode, *rmodel.Usage) {
	if meta == nil || meta.Mode != relaymode.ResponseAPI {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "invalid mode for response websocket handler", Type: rmodel.ErrorTypeOneAPI, Code: "invalid_mode", RawError: errors.New("invalid mode for response websocket handler")},
			StatusCode: http.StatusBadRequest,
		}, nil
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
		}, nil
	}
	defer func() { _ = clientConn.Close() }()

	fullRequestURL, err := resolveResponseAPIWebSocketUpstreamURL(c, meta)
	if err != nil {
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "resolve upstream url failed", Type: rmodel.ErrorTypeInternal, Code: "upstream_url_resolve_failed", RawError: err},
			StatusCode: http.StatusBadGateway,
		}, nil
	}

	requestHeader := http.Header{}
	if sp := c.GetHeader("Sec-WebSocket-Protocol"); sp != "" {
		requestHeader.Set("Sec-WebSocket-Protocol", sp)
	}
	requestHeader.Set("Authorization", "Bearer "+meta.APIKey)

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	upstreamConn, _, dialErr := dialer.Dial(fullRequestURL, requestHeader)
	if dialErr != nil {
		_ = clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "upstream connect failed"))
		return &rmodel.ErrorWithStatusCode{
			Error:      rmodel.Error{Message: "upstream response websocket connect failed: " + dialErr.Error(), Type: rmodel.ErrorTypeUpstream, Code: "upstream_connect_failed", RawError: dialErr},
			StatusCode: http.StatusBadGateway,
		}, nil
	}
	defer func() { _ = upstreamConn.Close() }()

	usage := &rmodel.Usage{}
	errc := make(chan error, 2)
	go func() { errc <- copyWS(clientConn, upstreamConn) }()
	go func() { errc <- copyResponseAPIWSUpstreamToClient(upstreamConn, clientConn, usage) }()

	if proxyErr := <-errc; proxyErr != nil {
		gmw.GetLogger(c).Debug("response websocket proxy closed", zap.String("error", proxyErr.Error()))
	}

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	return nil, usage
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

// copyResponseAPIWSUpstreamToClient forwards upstream frames to the client and
// extracts best-effort usage metrics from response events.
func copyResponseAPIWSUpstreamToClient(src, dst *websocket.Conn, usage *rmodel.Usage) error {
	countedResponseIDs := map[string]struct{}{}

	for {
		mt, msg, err := src.ReadMessage()
		if err != nil {
			if closeErr, ok := err.(*websocket.CloseError); ok {
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
			accumulateResponseAPIUsage(msg, usage, countedResponseIDs)
		}

		if werr := dst.WriteMessage(mt, msg); werr != nil {
			return errors.WithStack(werr)
		}
	}
}

// accumulateResponseAPIUsage parses one websocket text event and updates usage once
// per response ID to avoid double counting created/updated/completed snapshots.
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

	if event.Response == nil || event.Response.Usage == nil || event.Response.ID == "" {
		return "", nil, false
	}

	usage := &rmodel.Usage{
		PromptTokens:     event.Response.Usage.InputTokens,
		CompletionTokens: event.Response.Usage.OutputTokens,
		TotalTokens:      event.Response.Usage.TotalTokens,
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
