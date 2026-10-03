package model

// maxRoutingWeight caps a single candidate's contribution to the weighted-random
// total. Real deployments use tiny weights (single/double digits); the cap exists
// only so a pathologically large configured value can never overflow the int64
// accumulator and make the random draw panic. 2^40 is astronomically larger than
// any sane routing weight yet leaves headroom for millions of candidates before
// the running sum approaches math.MaxInt64.
const maxRoutingWeight = int64(1) << 40

// clampRoutingWeight converts a configured uint weight into a bounded int64. Any
// value above maxRoutingWeight (only reachable through absurd manual configuration)
// is clamped, so the accumulator in weightedIndex can never overflow.
func clampRoutingWeight(w uint) int64 {
	if uint64(w) > uint64(maxRoutingWeight) {
		return maxRoutingWeight
	}
	return int64(w)
}

// weightedIndex returns the index into weights chosen with probability
// proportional to each weight. It returns -1 when every weight is zero, signalling
// the caller to fall back to uniform selection.
//
// draw(n) must return a pseudo-random value in [0, n); math/rand's Int63n
// satisfies this and is what production callers pass. Injecting draw keeps the
// selection deterministic under test.
//
// The function never panics regardless of the configured weights: individual
// weights are clamped (clampRoutingWeight) and the total is validated to be
// strictly positive before draw is invoked.
func weightedIndex(weights []uint, draw func(n int64) int64) int {
	var total int64
	for _, w := range weights {
		total += clampRoutingWeight(w)
	}
	// total <= 0 covers both "all weights zero" and the theoretical accumulator
	// overflow (which clampRoutingWeight makes unreachable for any realistic
	// candidate count). Either way, uniform fallback is the safe behaviour.
	if total <= 0 {
		return -1
	}

	r := draw(total)
	var cumulative int64
	for i, w := range weights {
		cumulative += clampRoutingWeight(w)
		if r < cumulative {
			return i
		}
	}
	// Unreachable for any draw honouring [0, total): cumulative equals total after
	// the final element, and r < total. Returning the last index is a safe,
	// in-range fallback for a misbehaving draw rather than a panic.
	return len(weights) - 1
}
