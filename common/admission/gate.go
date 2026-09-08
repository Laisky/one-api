// Package admission bounds how many expensive operations a node runs at once.
//
// It exists because a time budget alone does not bound load: a hundred
// concurrent callers each allowed three seconds of database time is three
// hundred seconds of database time. The gate caps concurrency; the caller's
// context caps duration. Acquire the gate FIRST and then apply the timeout, so
// a caller that waits for a slot does not spend its query budget queueing.
//
// The implementation is a buffered channel and nothing else: no Redis, no
// shared state, no dependency on any other package. A node that cannot reach
// Redis still enforces its own limits.
package admission

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
)

// ErrBusy is returned when no slot became available within the wait budget.
var ErrBusy = errors.New("admission: no slot available")

// Gate limits concurrent entry to a named operation.
type Gate struct {
	name  string
	slots chan struct{}
	wait  time.Duration
}

// NewGate builds a gate.
//
// Parameters:
//   - name: the operation name, used in errors and metrics.
//   - capacity: how many callers may hold the gate at once; values below 1
//     become 1.
//   - wait: how long a caller waits for a slot before being refused.
//
// Return values:
//   - *Gate: the gate.
func NewGate(name string, capacity int, wait time.Duration) *Gate {
	if capacity < 1 {
		capacity = 1
	}
	return &Gate{name: name, slots: make(chan struct{}, capacity), wait: wait}
}

// Acquire takes a slot, waiting at most the gate's wait budget.
//
// Parameters:
//   - ctx: cancellation scope; a cancelled context refuses immediately.
//
// Return values:
//   - func(): releases the slot; nil when the gate was not entered.
//   - error: ErrBusy when no slot became available, or the context's error.
func (g *Gate) Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrapf(err, "admission: %s", g.name)
	}

	timer := time.NewTimer(g.wait)
	defer timer.Stop()

	select {
	case g.slots <- struct{}{}:
		var released bool
		return func() {
			if released {
				return
			}
			released = true
			<-g.slots
		}, nil
	case <-ctx.Done():
		return nil, errors.Wrapf(ctx.Err(), "admission: %s", g.name)
	case <-timer.C:
		return nil, errors.Wrapf(ErrBusy, "admission: %s", g.name)
	}
}

// Name returns the gate's operation name.
//
// Parameters: none.
//
// Return values:
//   - string: the name.
func (g *Gate) Name() string { return g.name }

// InFlight reports how many slots are currently held.
//
// Parameters: none.
//
// Return values:
//   - int: the number of held slots.
func (g *Gate) InFlight() int { return len(g.slots) }
