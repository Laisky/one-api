package gemini

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/realtime"
)

const liveMessageLimit = 4 << 20

// livePumpOptions bounds socket lifetime, writes and receipt draining. These are
// resource limits, not duration billing: only provider usage is priced.
type livePumpOptions struct {
	writeTimeout time.Duration
	drainTimeout time.Duration
	lifetime     time.Duration
}

// defaultLivePumpOptions returns production safety bounds. Parameters: none.
// Returns: bounded options; each connection is limited to fifteen minutes.
func defaultLivePumpOptions() livePumpOptions {
	return livePumpOptions{writeTimeout: 10 * time.Second, drainTimeout: 2 * time.Second, lifetime: 15 * time.Minute}
}

// LiveHandler proxies Gemini-native bidirectional JSON over /v1/realtime.
// Parameters: c is the authenticated request and m is the mapped channel.
// Returns: a pre-upgrade business error, or a sealed usage ledger after upgrade.
// Post-upgrade errors never masquerade as handshake failures that refund paid
// earlier turns. It shares the controller's reservations and durable settlement.
func LiveHandler(c *gin.Context, m *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
	endpoint, err := LiveRequestURL(m)
	if err != nil {
		return openai.ErrorWrapper(err, "gemini_live_configuration", http.StatusBadRequest), nil
	}
	if m.APIKey == "" {
		return openai.ErrorWrapper(errors.Wrap(ErrLiveProtocol, "missing channel API key"), "gemini_live_configuration", http.StatusBadRequest), nil
	}
	if !websocket.IsWebSocketUpgrade(c.Request) {
		return openai.ErrorWrapper(errors.Wrap(ErrLiveProtocol, "WebSocket upgrade required"), "gemini_live_upgrade", http.StatusBadRequest), nil
	}
	// Neither the caller's bearer token nor authentication subprotocol/query
	// parameters are ever copied to Google. Google SDKs use this key header.
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	upstream, response, err := dialer.DialContext(c.Request.Context(), endpoint, http.Header{"X-Goog-Api-Key": []string{m.APIKey}})
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return openai.ErrorWrapper(errors.Wrap(ErrLiveProtocol, "Gemini Live upstream handshake failed"), "gemini_live_connect", http.StatusBadGateway), nil
	}
	defer upstream.Close()
	// Keep the URL key-free in both metadata and diagnostics.
	m.UpstreamRequestURL = endpoint
	upgrader := websocket.Upgrader{HandshakeTimeout: 10 * time.Second}
	client, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade has already written an HTTP error. No setup/input reached the
		// provider, so return an authoritative zero ledger rather than write
		// another HTTP response or retain a fabricated estimate.
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	defer client.Close()
	client.SetReadLimit(liveMessageLimit)
	upstream.SetReadLimit(liveMessageLimit)
	_ = client.SetReadDeadline(time.Now().Add(10 * time.Second))
	kind, data, err := client.ReadMessage()
	if err != nil {
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	if kind != websocket.TextMessage && kind != websocket.BinaryMessage {
		liveClose(client, websocket.ClosePolicyViolation, "gemini_live_setup_required")
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	setup, err := prepareLiveSetup(data, m.ActualModelName, m.OriginModelName)
	if err != nil {
		liveClose(client, websocket.ClosePolicyViolation, "gemini_live_invalid_setup")
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	if err := liveWrite(upstream, websocket.TextMessage, setup, 10*time.Second); err != nil {
		liveClose(client, websocket.CloseTryAgainLater, "gemini_live_setup_failed")
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	_ = upstream.SetReadDeadline(time.Now().Add(10 * time.Second))
	kind, ack, err := upstream.ReadMessage()
	if err != nil {
		liveClose(client, websocket.CloseTryAgainLater, "gemini_live_setup_rejected")
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	var envelope map[string]json.RawMessage
	if err := realtime.ValidateGeminiJSON(ack); err != nil || json.Unmarshal(ack, &envelope) != nil || envelope["setupComplete"] == nil || string(envelope["setupComplete"]) == "null" {
		liveClose(client, websocket.CloseProtocolError, "gemini_live_setup_not_acknowledged")
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	if err := liveWrite(client, kind, ack, 10*time.Second); err != nil {
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	_ = client.SetReadDeadline(time.Time{})
	_ = upstream.SetReadDeadline(time.Time{})
	usage := runLivePump(client, upstream, defaultLivePumpOptions())
	lg.Debug("Gemini Live session finished", zap.Int("receipts", len(usage.Realtime.Records)),
		zap.Bool("usage_gap", usage.Realtime.HasUsageGap()), zap.Int("billing_issues", len(usage.Realtime.Issues)))
	return nil, usage
}

// liveWrite performs one bounded data write. Parameters: conn, kind, data and
// timeout describe the frame. Returns: a wrapped socket error.
func liveWrite(conn *websocket.Conn, kind int, data []byte, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return errors.Wrap(err, "set Live write deadline")
	}
	return errors.Wrap(conn.WriteMessage(kind, data), "write Live frame")
}

// liveClose sends a bounded control frame. Parameters: conn is a socket, code
// is a standard close code and reason is an internal non-secret diagnostic.
// Returns: none; socket teardown is authoritative even if peer already closed.
func liveClose(conn *websocket.Conn, code int, reason string) {
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
}

// liveLedgerUsage adapts the sealed ledger to existing quota/logging paths.
// Parameters: ledger is authoritative server evidence. Returns: token counters
// plus the ledger; no duration, text reconstruction or audio surcharge is added.
func liveLedgerUsage(ledger *realtime.Ledger) *model.Usage {
	return &model.Usage{Realtime: ledger, PromptTokens: int(ledger.InputTokens), CompletionTokens: int(ledger.OutputTokens), TotalTokens: int(ledger.InputTokens + ledger.OutputTokens)}
}
