package gemini

import (
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/gorilla/websocket"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/realtime"
)

// livePumpState contains only connection-scoped counters shared by the readers.
// It never retains Gin contexts, API keys, transcripts or audio payloads.
type livePumpState struct {
	clientGone atomic.Bool
	inputs     atomic.Int64
	receipted  atomic.Int64
	tools      liveToolState
}

// runLivePump joins both directional readers before exposing billing state.
// Parameters: client/upstream are acknowledged sockets and options bounds work.
// Returns: sealed usage. A lost downstream drains late upstream receipts for a
// bounded interval, without replaying input or keeping an unbounded paid session.
func runLivePump(client, upstream *websocket.Conn, options livePumpOptions) *model.Usage {
	collector := realtime.NewGeminiLedger()
	state := &livePumpState{}
	clientDone, serverDone := make(chan struct{}), make(chan struct{})
	go func() { defer close(clientDone); copyLiveClient(client, upstream, state, options) }()
	go func() { defer close(serverDone); copyLiveServer(upstream, client, state, collector, options) }()
	lifetime := time.NewTimer(options.lifetime)
	defer lifetime.Stop()
	select {
	case <-clientDone:
		state.clientGone.Store(true)
		_ = client.Close()
		drain := time.NewTimer(options.drainTimeout)
		select {
		case <-serverDone:
		case <-drain.C:
		case <-lifetime.C:
		}
		drain.Stop()
	case <-serverDone:
	case <-lifetime.C:
		liveClose(client, websocket.CloseNormalClosure, "gemini_live_session_time_limit")
	}
	_ = upstream.Close()
	_ = client.Close()
	<-clientDone
	<-serverDone
	return liveLedgerUsage(collector.Finish(state.inputs.Load() > state.receipted.Load()))
}

// copyLiveClient validates client operations before forwarding them. Parameters:
// sockets, state and options describe the session. Returns: none at disconnect
// or protocol error. Client-supplied usage can never enter the billing collector.
func copyLiveClient(client, upstream *websocket.Conn, state *livePumpState, options livePumpOptions) {
	defer state.clientGone.Store(true)
	for {
		kind, data, err := client.ReadMessage()
		if err != nil {
			return
		}
		if kind != websocket.TextMessage && kind != websocket.BinaryMessage {
			liveClose(client, websocket.ClosePolicyViolation, "gemini_live_json_required")
			return
		}
		work, err := validateLiveClientFrame(data)
		if err != nil {
			liveClose(client, websocket.ClosePolicyViolation, "gemini_live_invalid_client_frame")
			return
		}
		toolResponse, err := state.tools.accept(data)
		if err != nil {
			liveClose(client, websocket.ClosePolicyViolation, "gemini_live_unrequested_tool_response")
			return
		}
		if toolResponse {
			work = false
		}
		// Count before the write: a failed write can still have reached Google.
		// This is a pending-evidence marker, not a client-supplied usage amount.
		if work {
			state.inputs.Add(1)
		}
		if err := liveWrite(upstream, kind, data, options.writeTimeout); err != nil {
			return
		}
	}
}

// copyLiveServer observes usage before forwarding every upstream frame. Parameters:
// sockets, state, collector and options describe the session. Returns: none when
// upstream ends or the bounded receipt ledger fills. It preserves interruption,
// transcription, tool cancellation, goAway and IN_PROGRESS/IDLE native events.
func copyLiveServer(upstream, client *websocket.Conn, state *livePumpState, collector *realtime.GeminiLedger, options livePumpOptions) {
	var turnInputs int64
	turnStarted := false
	for {
		kind, data, err := upstream.ReadMessage()
		if err != nil {
			if errors.Is(err, websocket.ErrReadLimit) {
				_ = collector.MarkIncomplete("Gemini upstream frame exceeded the message limit")
			}
			if !state.clientGone.Load() {
				code := websocket.CloseInternalServerErr
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					code = websocket.CloseNormalClosure
				}
				liveClose(client, code, "gemini_live_upstream_closed")
			}
			return
		}
		if kind != websocket.TextMessage && kind != websocket.BinaryMessage {
			_ = collector.MarkIncomplete("Gemini upstream sent a non-JSON frame")
			return
		}
		if err := realtime.ValidateGeminiJSON(data); err != nil {
			_ = collector.MarkIncomplete("Gemini upstream sent invalid JSON")
			return
		}
		var event struct {
			Usage   json.RawMessage `json:"usageMetadata"`
			Content *struct {
				Model json.RawMessage `json:"modelTurn"`
			} `json:"serverContent"`
			Tool json.RawMessage `json:"toolCall"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			_ = collector.MarkIncomplete("invalid Gemini server envelope")
			return
		}
		if !turnStarted && (event.Usage != nil || event.Tool != nil || (event.Content != nil && event.Content.Model != nil)) {
			turnInputs = state.inputs.Load()
			turnStarted = true
		}
		if err := state.tools.observe(data); err != nil {
			_ = collector.MarkIncomplete("Gemini function protocol limit")
			return
		}
		before := len(collector.Ledger.Records)
		meterErr := collector.Observe(data)
		if len(collector.Ledger.Records) > before {
			state.receipted.Store(turnInputs)
			turnStarted = false
		}
		if !state.clientGone.Load() {
			if err := liveWrite(client, kind, data, options.writeTimeout); err != nil {
				state.clientGone.Store(true)
				_ = client.Close()
			}
		}
		if meterErr != nil {
			_ = collector.MarkIncomplete("Gemini metering stopped the session")
			liveClose(client, websocket.ClosePolicyViolation, "gemini_live_billing_capacity")
			return
		}
		if state.clientGone.Load() && state.inputs.Load() <= state.receipted.Load() && !collector.Ledger.HasUsageGap() {
			return
		}
	}
}
