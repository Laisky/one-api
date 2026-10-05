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
	coverage   *liveToolCoverage
	spend      *liveSpend
}

// hasPendingInput reports forwarded client work without a covering receipt.
// Parameters: none. Returns: true for user input or function results that no
// receipt provably covers (see liveToolCoverage).
func (s *livePumpState) hasPendingInput() bool {
	return s.inputs.Load() > s.receipted.Load() || s.coverage.pending()
}

// runLivePump joins both directional readers before exposing billing state.
// Parameters: client/upstream are acknowledged sockets, options bounds work,
// spend funds every forwarded operation and blocking lists the functions the
// provider awaits. Returns: sealed usage. A lost downstream drains late
// upstream receipts for a bounded interval, without replaying input or keeping
// an unbounded or unfunded paid session.
func runLivePump(client, upstream *websocket.Conn, options livePumpOptions, spend *liveSpend, blocking map[string]bool) *model.Usage {
	collector := realtime.NewGeminiLedger()
	state := &livePumpState{spend: spend, coverage: newLiveToolCoverage(blocking)}
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
	return liveLedgerUsage(collector.Finish(state.hasPendingInput()))
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
		calls, err := state.tools.accept(data)
		if err != nil {
			liveClose(client, websocket.ClosePolicyViolation, "gemini_live_unrequested_tool_response")
			return
		}
		toolResponse := calls != nil
		if toolResponse {
			work = false
		}
		// Fund every operation before it can reach the provider: inputs are
		// bounded from the wire, and controls or function results may start a
		// turn that re-bills the context. Unfunded work is never forwarded.
		input, err := estimateLiveClientFrame(data)
		if err == nil {
			err = state.spend.admit(input)
		}
		if err != nil {
			liveClose(client, websocket.ClosePolicyViolation, state.spend.refuse(err))
			return
		}
		// Count before the write: a failed write can still have reached Google.
		// This is a pending-evidence marker, not a client-supplied usage amount.
		if work {
			state.inputs.Add(1)
		}
		if toolResponse {
			state.coverage.admit(calls)
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
	// endedUnreceipted marks a turn that ended without its usage receipt until
	// a late receipt commits it or the next turn's model work starts.
	endedUnreceipted := false
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
				Model       json.RawMessage `json:"modelTurn"`
				Output      json.RawMessage `json:"outputTranscription"`
				Complete    bool            `json:"turnComplete"`
				Interrupted bool            `json:"interrupted"`
				Status      string          `json:"interactionStatus"`
			} `json:"serverContent"`
			Tool   json.RawMessage `json:"toolCall"`
			Cancel json.RawMessage `json:"toolCallCancellation"`
			Status string          `json:"interactionStatus"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			_ = collector.MarkIncomplete("invalid Gemini server envelope")
			return
		}
		// Any model work starts a turn, including an output transcription that
		// arrives before the model's own frames, so the boundary is never late.
		if !turnStarted && (event.Usage != nil || event.Tool != nil || (event.Content != nil && (event.Content.Model != nil || event.Content.Output != nil))) {
			turnInputs = state.inputs.Load()
			state.coverage.turnStarted()
			turnStarted = true
		}
		if err := state.tools.observe(data, state.coverage.currentTurn()); err != nil {
			_ = collector.MarkIncomplete("Gemini function protocol limit")
			return
		}
		if event.Cancel != nil || (event.Content != nil && event.Content.Interrupted) {
			state.coverage.interrupt()
		}
		if event.Content != nil && event.Content.Model != nil {
			state.coverage.modelOutput()
		}
		if endedUnreceipted && (event.Tool != nil || (event.Content != nil && event.Content.Model != nil)) {
			// The previous turn ended without usage and this frame starts the
			// next one: its incurred work must survive the next turn's receipt.
			state.spend.rolloverUnreceiptedTurn()
			endedUnreceipted = false
		}
		awaitingReceipt := collector.AwaitingReceipt()
		before := len(collector.Ledger.Records)
		meterErr := collector.Observe(data)
		if len(collector.Ledger.Records) > before {
			endedUnreceipted = false
		} else if !awaitingReceipt && collector.AwaitingReceipt() {
			endedUnreceipted = true
		}
		if len(collector.Ledger.Records) > before {
			state.receipted.Store(turnInputs)
			state.coverage.receipted()
			state.coverage.turnEnded()
			turnStarted = false
		} else if event.Status == "IDLE" || (event.Content != nil && (event.Content.Complete || event.Content.Status == "IDLE")) {
			// The turn ended before its receipt: results for its calls that
			// arrive from now on can only be consumed by a later turn.
			state.coverage.turnEnded()
		}
		// Stop paid generation that the reservation no longer covers. The frame
		// is withheld; already committed receipts stay authoritative.
		modelWork := event.Usage != nil || event.Tool != nil ||
			(event.Content != nil && (event.Content.Model != nil || event.Content.Output != nil))
		if err := state.spend.observe(data, modelWork, collector.Ledger.Records[before:]); err != nil {
			if meterErr != nil {
				_ = collector.MarkIncomplete("Gemini metering stopped the session")
			}
			if !state.clientGone.Load() {
				liveClose(client, websocket.ClosePolicyViolation, liveSpendCloseReason(err))
			}
			return
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
		// A lost downstream keeps draining until the upstream closes or the
		// bounded drain ends: a generation the provider starts for work that a
		// receipt seemed to cover is still observed and billed, never cut off.
	}
}
