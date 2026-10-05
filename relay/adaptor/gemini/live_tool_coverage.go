package gemini

import "sync"

// liveToolCoverage decides which forwarded function results a turn's usage
// receipt may cover. A result only counts as receipted when the provider must
// have consumed it before producing that receipt:
//   - a result of an explicitly BLOCKING function answering a call of the
//     still-open turn is covered by that turn's receipt once model output is
//     read after the result was admitted. The provider pauses a blocking call
//     until its response, so that output proves the response was consumed;
//     without it (an interruption, a cancellation, or a turn that closed
//     before the response) the result is deferred;
//   - every other result (a non-blocking function under any scheduling, a call
//     whose turn already closed) joins the context of a later generation, so
//     only the receipt of a turn whose first frame was read after the result
//     was admitted covers it. This is the same turn-start boundary that assigns
//     user input and funded work to turns.
//
// Anything not covered stays pending, so the session settles at the funded
// input estimate: an ambiguous result may be over-billed, never served
// unbilled. The mutex serializes both socket readers.
type liveToolCoverage struct {
	mu          sync.Mutex
	blocking    map[string]bool
	turn        int64 // sequence of the open turn; every turn end advances it
	interrupted bool  // the open turn was interrupted or cancelled a call
	awaited     int64 // blocking results of the open turn without later output
	confirmed   int64 // blocking results the provider demonstrably consumed
	waiting     int64 // deferred results that need a turn starting after them
	eligible    int64 // deferred results admitted before the open turn began
}

// newLiveToolCoverage creates the tracker. Parameters: blocking lists the
// functions the provider waits for (see liveBlockingFunctions). Returns: an
// empty tracker for one connection.
func newLiveToolCoverage(blocking map[string]bool) *liveToolCoverage {
	return &liveToolCoverage{blocking: blocking}
}

// currentTurn returns the sequence assigned to calls of the open turn.
// Parameters: none. Returns: the turn sequence.
func (c *liveToolCoverage) currentTurn() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turn
}

// admit classifies and counts one function-result frame before it is written
// upstream, so a failed write that still reached the provider stays pending.
// Parameters: calls are the server calls the frame answers. Returns: none.
func (c *liveToolCoverage) admit(calls []liveCall) {
	c.mu.Lock()
	defer c.mu.Unlock()
	awaited := !c.interrupted && len(calls) > 0
	for _, call := range calls {
		if !c.blocking[call.name] || call.turn != c.turn {
			awaited = false
		}
	}
	if awaited {
		c.awaited++
	} else {
		c.waiting++
	}
}

// modelOutput records model output read in the open turn. Parameters: none.
// Returns: none. Blocking results admitted before it are now consumed, unless
// the turn was interrupted, after which output no longer proves consumption.
func (c *liveToolCoverage) modelOutput() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.interrupted {
		return
	}
	c.confirmed += c.awaited
	c.awaited = 0
}

// turnStarted makes deferred results admitted before the new turn's first
// frame was read coverable by that turn's receipt. Parameters: none. Returns:
// none.
func (c *liveToolCoverage) turnStarted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eligible += c.waiting
	c.waiting = 0
}

// interrupt marks the open turn as ended without awaiting every result.
// Parameters: none. Returns: none.
func (c *liveToolCoverage) interrupt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interrupted = true
}

// turnEnded closes the open turn, so later results for its calls are deferred.
// Parameters: none. Returns: none. It runs on turnComplete, an idle status or a
// committed receipt, whichever the provider sends first; extra calls only make
// later results more conservative.
func (c *liveToolCoverage) turnEnded() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turn++
}

// receipted applies one committed usage receipt. Parameters: none. Returns:
// none. It covers consumed blocking results and deferred results admitted
// before the receipted turn began; blocking results the turn never consumed
// wait for a turn that starts after this receipt.
func (c *liveToolCoverage) receipted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.confirmed, c.eligible = 0, 0
	c.waiting += c.awaited
	c.awaited = 0
	c.interrupted = false
}

// pending reports forwarded function results without a covering receipt.
// Parameters: none. Returns: true while any result is unreceipted.
func (c *liveToolCoverage) pending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.awaited+c.confirmed+c.waiting+c.eligible > 0
}
