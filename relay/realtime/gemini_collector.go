package realtime

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// GeminiLedger collects server-only Live usage snapshots into the shared billing
// ledger. It is single-writer; inspect it only after the upstream reader stops.
// Unlike OpenAI, Google does not publish a response ID on each receipt, so
// duplicate suppression is limited to the current server-delimited turn.
type GeminiLedger struct {
	Ledger     *Ledger
	pending    *Record
	last       *Record
	active     bool
	waiting    bool
	inProgress bool
	invalid    bool
	finished   bool
	unanchored bool
}

// NewGeminiLedger creates an independent collector. Parameters: none. Returns:
// an idle ledger, for which connecting without sending work incurs no charge.
func NewGeminiLedger() *GeminiLedger { return &GeminiLedger{Ledger: NewLedger()} }

// Observe accepts only a trusted upstream JSON frame. Parameters: message is the
// frame. Returns: a classifiable error; valid earlier records remain billable.
// Input/output transcription deltas are never separately tokenized or charged.
func (g *GeminiLedger) Observe(message []byte) error {
	if g.finished || g.Ledger.stopped {
		return errors.WithStack(ErrLedgerLimit)
	}
	if err := ValidateGeminiJSON(message); err != nil {
		return g.MarkIncomplete("invalid or ambiguous Gemini server JSON")
	}
	var event struct {
		Usage   json.RawMessage `json:"usageMetadata"`
		Content *struct {
			Model       json.RawMessage `json:"modelTurn"`
			Input       json.RawMessage `json:"inputTranscription"`
			Interim     json.RawMessage `json:"interimInputTranscription"`
			Output      json.RawMessage `json:"outputTranscription"`
			Complete    bool            `json:"turnComplete"`
			Interrupted bool            `json:"interrupted"`
			Status      string          `json:"interactionStatus"`
		} `json:"serverContent"`
		Tool   json.RawMessage `json:"toolCall"`
		Status string          `json:"interactionStatus"`
	}
	if err := json.Unmarshal(message, &event); err != nil {
		return g.MarkIncomplete("invalid Gemini server event")
	}
	// A new model turn after a completed receipt starts a new deduplication
	// scope. Equal token counts on two different turns are both chargeable.
	modelWork := event.Tool != nil || (event.Content != nil && event.Content.Model != nil)
	if modelWork && !g.active {
		if g.unanchored {
			g.pending, g.unanchored = nil, false
			_ = g.MarkIncomplete("Gemini usage snapshot had no attributable turn")
		}
		if g.waiting {
			_ = g.MarkIncomplete("Gemini turn ended without usage")
		}
		g.last, g.waiting = nil, false
		g.active = true
	}
	wasInProgress := g.inProgress
	status := event.Status
	if event.Content != nil && event.Content.Status != "" {
		status = event.Content.Status
	}
	if status == "IN_PROGRESS" {
		g.inProgress = true
	}
	if status == "IDLE" {
		g.inProgress = false
	}
	if event.Content != nil && (event.Content.Input != nil || event.Content.Output != nil || event.Content.Interim != nil || event.Content.Interrupted) {
		// Transcriptions can arrive out of order. They indicate work, but not a
		// new charge or a turn boundary. The provider's TEXT receipt pays them.
		if g.last == nil {
			g.active = true
		}
	}
	// An independent IDLE event, without a second spoken turnComplete, also
	// ends Extended Thinking background work. Repeated idle notifications do
	// not create phantom turns. A later receipt can fill the waiting boundary.
	terminal := (event.Content != nil && event.Content.Complete && !g.inProgress) ||
		(status == "IDLE" && wasInProgress)
	var observationErr error
	if event.Usage != nil {
		r, err := DecodeGeminiUsage(event.Usage)
		if err != nil {
			g.invalid = true
			observationErr = g.Ledger.recordIssue(err)
		} else {
			g.invalid = false
			if g.last != nil && !g.active && !g.waiting && !terminal {
				// Do not globally deduplicate equal token counts. Stage the
				// snapshot until a new turn boundary identifies its scope.
				g.unanchored = true
			}
			if g.pending != nil && !geminiSnapshotExtends(g.pending.Tokens, r.Tokens) {
				return g.MarkIncomplete("Gemini usage regressed within a turn")
			}
			g.pending = &r
			if g.waiting {
				return g.commit()
			}
		}
	}
	if terminal {
		if g.pending != nil {
			if err := g.commit(); err != nil {
				return err
			}
			// Committing earlier valid usage must not hide an invalid final
			// receipt from the transport's stop-on-metering-error boundary.
			return observationErr
		}
		g.waiting, g.active = true, false
	}
	return observationErr
}

// geminiSnapshotExtends checks cumulative refinements within one turn only.
// Parameters: old and next are validated counters. Returns: true for monotonic
// partitions, including exact duplicate snapshots. Context shrink across turns
// is intentionally not subject to this check.
func geminiSnapshotExtends(old, next Tokens) bool {
	return next.Text >= old.Text && next.Audio >= old.Audio && next.Image >= old.Image && next.Video >= old.Video &&
		next.OutputText >= old.OutputText && next.OutputAudio >= old.OutputAudio && next.ReasoningTokens >= old.ReasoningTokens
}

// commit records the current turn once. Parameters: none. Returns: an error at
// the ledger capacity; the final accepted receipt is retained before stopping.
func (g *GeminiLedger) commit() error {
	if g.pending == nil {
		return nil
	}
	if err := g.Ledger.appendRecord(*g.pending); err != nil {
		return err
	}
	if g.invalid {
		// A valid snapshot in a later turn cannot repair this rejected final
		// receipt. Keep measured work, but preserve its reconciliation gap.
		_ = g.MarkIncomplete("Gemini turn committed with a rejected usage receipt")
		g.invalid = false
	}
	g.last, g.pending = g.pending, nil
	g.active, g.waiting, g.unanchored = false, false, false
	if len(g.Ledger.Records) >= MaxRecords {
		return g.Ledger.stop()
	}
	return nil
}

// MarkIncomplete records a payload-free reconciliation diagnostic. Parameters:
// reason is an internal diagnostic, never provider text. Returns: the issue.
func (g *GeminiLedger) MarkIncomplete(reason string) error {
	g.Ledger.unkeyedUsageGap = true
	return g.Ledger.issue(reason)
}

// Finish seals the collector after both socket pumps join. Parameters: pendingInput
// reports client work sent after the last observed final turn. Returns: the
// shared receipt ledger. Missing evidence keeps the existing reservation floor;
// partial measured work is not discarded, and an explicit idle session is free.
func (g *GeminiLedger) Finish(pendingInput bool) *Ledger {
	if g.finished {
		return g.Ledger
	}
	g.finished = true
	if g.unanchored {
		g.pending = nil
		_ = g.MarkIncomplete("Gemini usage snapshot had no attributable final turn")
	}
	if g.pending != nil {
		_ = g.commit()
		_ = g.MarkIncomplete("Gemini connection ended before final turn boundary")
	}
	if g.active || g.waiting || g.inProgress || g.invalid || pendingInput {
		_ = g.MarkIncomplete("Gemini connection ended with unresolved work")
	}
	return g.Ledger
}
