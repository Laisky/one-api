package gemini

import (
	"sync"

	"github.com/Laisky/errors/v2"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/realtime"
)

// Close reasons for funding decisions. They are diagnostics, not user payloads.
const (
	liveCloseQuotaExhausted  = "gemini_live_quota_exhausted"
	liveCloseUnpriceableWork = "gemini_live_unpriceable_input"
)

// SetLiveSpendGate attaches the prepaid session budget that every Live
// forwarding decision must consult. Parameters: c is the request context and
// gate is a non-nil controller-owned budget. Returns: none.
func SetLiveSpendGate(c *gin.Context, gate realtime.SpendGate) {
	c.Set(ctxkey.RealtimeSpendGate, gate)
}

// liveSpendGateFrom returns the attached budget. Parameters: c is the request
// context. Returns: nil when no budget is attached; Live then fails closed.
func liveSpendGateFrom(c *gin.Context) realtime.SpendGate {
	value, ok := c.Get(ctxkey.RealtimeSpendGate)
	if !ok {
		return nil
	}
	gate, _ := value.(realtime.SpendGate)
	return gate
}

// liveSpend keeps a session's prepaid reservation ahead of its provider work.
// Gemini Live re-bills the whole conversation context on every turn and streams
// output before the per-turn receipt, so the exposure is modeled per turn:
//   - an in-flight turn costs its context, its inputs, the output streamed so
//     far and an output allowance for hidden thinking and the next frame;
//   - client work forwarded since that turn started can start one more turn,
//     whose context also contains the in-flight turn and its output.
//
// Both socket readers share the tracker; its mutex serializes every funding
// decision so that one reservation can never fund two forwarded frames.
type liveSpend struct {
	mu              sync.Mutex
	lg              glog.Logger
	gate            realtime.SpendGate
	allowance       int64
	carried         int64
	pending         realtime.Estimate
	pendingWork     bool
	inflight        bool
	inflightContext int64
	inflightInput   realtime.Estimate
	inflightOutput  realtime.Estimate
	unreceipted     realtime.Estimate
	exhausted       bool
}

// newLiveSpend creates the tracker after setup validation. Parameters: gate is
// the session budget, setup the validated, model-pinned setup frame and lg the
// request logger value (it never retains the gin context). Returns: a tracker
// whose first turn re-bills the setup context, or an error wrapping
// realtime.ErrUnpriceableInput for setup content that cannot be bounded.
func newLiveSpend(gate realtime.SpendGate, setup []byte, lg glog.Logger) (*liveSpend, error) {
	estimate, err := estimateLiveContent(setup)
	if err != nil {
		return nil, err
	}
	return &liveSpend{lg: lg, gate: gate, allowance: liveTurnOutputAllowance(setup), carried: estimate.InputTokens()}, nil
}

// refuse records why a client operation was not forwarded. Parameters: err is
// the estimation or funding error. Returns: the close reason for the client.
// Budget exhaustion is already logged by the gate with its balances.
func (s *liveSpend) refuse(err error) string {
	reason := liveSpendCloseReason(err)
	if reason != liveCloseQuotaExhausted && s.lg != nil {
		s.lg.Warn("Gemini Live client operation refused before forwarding", zap.String("close_reason", reason), zap.Error(err))
	}
	return reason
}

// exposureLocked returns the funding bound of all unreceipted work. Parameters:
// pending and pendingWork describe client work forwarded since the in-flight
// turn started. Returns: an estimate; the caller holds s.mu.
func (s *liveSpend) exposureLocked(pending realtime.Estimate, pendingWork bool) realtime.Estimate {
	// Turns that ended without a receipt are incurred work no receipt will
	// ever commit, so they stay funded until settlement.
	exposure := s.unreceipted
	nextContext := s.carried
	if s.inflight {
		turn := s.inflightInput.Add(realtime.Estimate{Context: s.inflightContext, Output: s.allowance,
			OutputText: s.inflightOutput.OutputText, OutputAudio: s.inflightOutput.OutputAudio})
		exposure = exposure.Add(turn)
		nextContext = turn.TotalTokens()
	}
	if pendingWork {
		exposure = exposure.Add(pending.Add(realtime.Estimate{Context: nextContext, Output: s.allowance}))
	}
	return exposure
}

// admit funds one client operation before it is forwarded. Parameters: input
// bounds the operation's own tokens; controls carry none but may start a turn.
// Returns: nil when funded, or an error wrapping realtime.ErrBudgetExhausted.
// A refused operation leaves the tracker unchanged and must never be forwarded.
func (s *liveSpend) admit(input realtime.Estimate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exhausted {
		return errors.WithStack(realtime.ErrBudgetExhausted)
	}
	pending := s.pending.Add(input)
	if err := s.gate.Ensure(s.exposureLocked(pending, true)); err != nil {
		s.exhausted = true
		return errors.Wrap(err, "fund Live client operation")
	}
	s.pending, s.pendingWork = pending, true
	return nil
}

// observe accounts for one trusted server frame before it is forwarded.
// Parameters: data is the frame, modelWork reports model output or a receipt,
// and receipts are the records the collector committed for this frame.
// Returns: nil while the session remains funded, or an error wrapping
// realtime.ErrBudgetExhausted; the caller must then stop the provider session.
func (s *liveSpend) observe(data []byte, modelWork bool, receipts []realtime.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if modelWork && !s.inflight {
		s.inflight, s.inflightContext, s.inflightInput = true, s.carried, s.pending
		s.inflightOutput, s.pending, s.pendingWork = realtime.Estimate{}, realtime.Estimate{}, false
	}
	if s.inflight {
		s.inflightOutput = s.inflightOutput.Add(estimateLiveServerOutput(data))
	}
	for _, receipt := range receipts {
		s.gate.Commit(receipt)
		// The provider's own counts replace every estimate of this turn. Its
		// prompt and output both re-enter the context of the next turn.
		// Validated receipts guarantee Input+Output does not overflow.
		s.carried = receipt.Tokens.Input + receipt.Tokens.Output
		s.inflight, s.inflightContext = false, 0
		s.inflightInput, s.inflightOutput = realtime.Estimate{}, realtime.Estimate{}
	}
	if s.exhausted {
		return errors.WithStack(realtime.ErrBudgetExhausted)
	}
	if err := s.gate.Ensure(s.exposureLocked(s.pending, s.pendingWork)); err != nil {
		s.exhausted = true
		return errors.Wrap(err, "fund Live provider work")
	}
	return nil
}

// rolloverUnreceiptedTurn keeps the evidence of a turn the provider ended
// without a usage receipt when the next turn starts, so a later receipt for
// that next turn cannot erase it. Parameters: none. Returns: none. The ended
// turn's context, inputs and streamed output become permanent evidence and
// the next turn's context.
func (s *liveSpend) rolloverUnreceiptedTurn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.inflight {
		return
	}
	turn := s.inflightEvidenceLocked()
	s.unreceipted = s.unreceipted.Add(turn)
	s.carried = turn.TotalTokens()
	s.inflight, s.inflightContext = false, 0
	s.inflightInput, s.inflightOutput = realtime.Estimate{}, realtime.Estimate{}
}

// inflightEvidenceLocked returns the work the in-flight turn demonstrably
// incurred: its context, inputs and streamed output, without the allowance.
// Parameters: none. Returns: the estimate; the caller holds s.mu.
func (s *liveSpend) inflightEvidenceLocked() realtime.Estimate {
	return s.inflightInput.Add(realtime.Estimate{Context: s.inflightContext,
		OutputText: s.inflightOutput.OutputText, OutputAudio: s.inflightOutput.OutputAudio})
}

// finish reports the unreceipted work the provider demonstrably received.
// Parameters: none. Returns: none. Allowances for turns that never started are
// excluded; an in-flight turn keeps its context, inputs and streamed output,
// and turns that ended without a receipt keep theirs.
func (s *liveSpend) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	evidence := s.pending.Add(s.unreceipted)
	if s.inflight {
		evidence = evidence.Add(s.inflightEvidenceLocked())
	}
	s.gate.Finish(evidence)
}

// isExhausted reports whether a funding decision stopped the session.
// Parameters: none. Returns: true after any refused operation or frame.
func (s *liveSpend) isExhausted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exhausted
}

// liveSpendCloseReason maps a refused operation to a stable close reason.
// Parameters: err is a funding or estimation error. Returns: the reason.
func liveSpendCloseReason(err error) string {
	if errors.Is(err, realtime.ErrUnpriceableInput) {
		return liveCloseUnpriceableWork
	}
	if errors.Is(err, realtime.ErrBudgetExhausted) {
		return liveCloseQuotaExhausted
	}
	return "gemini_live_invalid_client_frame"
}
