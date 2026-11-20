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

// LiveTransport carries provider-owned connection settings into the shared
// native protocol handler. Headers must contain upstream credentials only;
// ModelResource is constructed from the authenticated channel, never client input.
type LiveTransport struct {
	Endpoint      string
	Headers       http.Header
	ModelResource string
}

// LiveHandler configures the Gemini Developer API transport. Parameters: c is
// authenticated and m carries the channel mapping. Returns: a pre-upgrade error
// or the sealed receipt ledger. Vertex uses its own OAuth transport instead.
func LiveHandler(c *gin.Context, m *meta.Meta) (*model.ErrorWithStatusCode, *model.Usage) {
	endpoint, err := LiveRequestURL(m)
	if err != nil {
		return openai.ErrorWrapper(err, "gemini_live_configuration", http.StatusBadRequest), nil
	}
	if m.APIKey == "" {
		return openai.ErrorWrapper(errors.Wrap(ErrLiveProtocol, "missing channel API key"), "gemini_live_configuration", http.StatusBadRequest), nil
	}
	return LiveHandlerWithTransport(c, m, LiveTransport{
		Endpoint: endpoint, Headers: http.Header{"X-Goog-Api-Key": []string{m.APIKey}},
		ModelResource: "models/" + m.ActualModelName,
	})
}

// LiveHandlerWithTransport shares protocol validation and receipt accounting
// across Google's backends. Parameters: c and m identify the authorized session;
// transport supplies validated provider settings. Returns: a pre-upgrade error
// or usage after both socket readers have joined. It never replays paid input.
func LiveHandlerWithTransport(c *gin.Context, m *meta.Meta, transport LiveTransport) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
	if m == nil || transport.Endpoint == "" || transport.ModelResource == "" {
		return openai.ErrorWrapper(errors.Wrap(ErrLiveProtocol, "missing Live transport configuration"), "gemini_live_configuration", http.StatusBadRequest), nil
	}
	if !websocket.IsWebSocketUpgrade(c.Request) {
		return openai.ErrorWrapper(errors.Wrap(ErrLiveProtocol, "WebSocket upgrade required"), "gemini_live_upgrade", http.StatusBadRequest), nil
	}
	// Fail closed: without a prepaid budget no billable frame may be forwarded.
	gate := liveSpendGateFrom(c)
	if gate == nil {
		return openai.ErrorWrapper(errors.Wrap(ErrLiveProtocol, "missing Live session budget"), "gemini_live_configuration", http.StatusInternalServerError), nil
	}
	// The endpoint is administrator-owned (ResolveLiveEndpoint already refused
	// userinfo, query and fragment); the shared dial-site policy re-checks the
	// final string before any credential is sent to it.
	endpoint, err := openai.ValidateProviderWebSocketURL(transport.Endpoint)
	if err != nil {
		// The policy error is handled here by reporting the same fixed,
		// URL-free configuration error as ResolveLiveEndpoint.
		return openai.ErrorWrapper(errors.Wrap(ErrLiveProtocol, "invalid Gemini Live endpoint"), "gemini_live_configuration", http.StatusBadRequest), nil
	}
	// Never copy caller headers, authentication subprotocols, or query values.
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	upstream, response, err := dialer.DialContext(c.Request.Context(), endpoint, transport.Headers.Clone())
	if err != nil {
		// Releases the handshake response (a close failure is DEBUG-logged, never
		// ignored) and reports the provider's 400..599 status, or 502 otherwise.
		status, providerStatus := openai.WebSocketHandshakeFailureStatus(lg, response)
		// Preserve the upstream status, including actual IAM/availability denials,
		// without leaking credential-bearing URLs or untrusted response bodies.
		connectErr := errors.Wrapf(ErrLiveProtocol, "Google Live upstream handshake failed (HTTP %d)", status)
		if providerStatus {
			// The provider answered the handshake with this status: attribute it
			// to the channel so 429/5xx drive the adaptive ability cooldown.
			return openai.UpstreamErrorWrapper(connectErr, "gemini_live_connect", status), nil
		}
		// No provider status (dial/transport failure or a non-error answer):
		// the 502 is one-api's own.
		return openai.ErrorWrapper(connectErr, "gemini_live_connect", status), nil
	}
	defer upstream.Close()
	m.UpstreamRequestURL = endpoint
	upgrader := websocket.Upgrader{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     openai.NegotiateRealtimeSubprotocols(c.Request),
	}
	client, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
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
	setup, err := prepareLiveSetupForResource(data, m.ActualModelName, m.OriginModelName, transport.ModelResource)
	if err != nil {
		liveClose(client, websocket.ClosePolicyViolation, "gemini_live_invalid_setup")
		return nil, liveLedgerUsage(realtime.NewLedger())
	}
	spend, err := newLiveSpend(gate, setup, lg)
	if err != nil {
		lg.Warn("Gemini Live setup cannot be priced before forwarding", zap.Error(err))
		liveClose(client, websocket.ClosePolicyViolation, liveSpendCloseReason(err))
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
	usage := runLivePump(client, upstream, defaultLivePumpOptions(), spend, liveBlockingFunctions(setup))
	spend.finish()
	lg.Debug("Gemini Live session finished", zap.Int("receipts", len(usage.Realtime.Records)),
		zap.Bool("usage_gap", usage.Realtime.HasUsageGap()), zap.Int("billing_issues", len(usage.Realtime.Issues)),
		zap.Bool("budget_exhausted", spend.isExhausted()))
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
