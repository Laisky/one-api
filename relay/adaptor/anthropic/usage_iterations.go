package anthropic

import (
	"encoding/json"
	"math"

	"github.com/Laisky/errors/v2"
)

// httpUsageIteration keeps one chronological cumulative iteration receipt.
// Parameters: kind identifies the provider iteration and receipt retains counters.
// This private value has no public serialization or caller-controlled authority.
type httpUsageIteration struct {
	kind    string
	receipt httpUsageReceipt
	// updated records presence in this snapshot rather than inherited counters.
	updated httpUsageUpdate
}

// sumHTTPUsageIterations validates and sums one canonical cumulative snapshot.
// Parameters: raw is usage.iterations and previous retains partial indexed fields.
// Returns: canonical totals, independent iteration states, or an atomic error.
// Anthropic's compaction-threshold usage contract includes both compaction and
// message iterations; adding root totals would bill message work a second time.
// https://platform.claude.com/docs/en/build-with-claude/compaction-threshold#understanding-usage
func sumHTTPUsageIterations(raw json.RawMessage, previous []httpUsageIteration) (httpUsageReceipt, []httpUsageIteration, error) {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return httpUsageReceipt{}, nil, errors.Wrap(err, "decode Claude usage iterations")
	}
	if len(entries) == 0 {
		return httpUsageReceipt{}, nil, errors.New("Claude usage iterations has no counters")
	}
	states := make([]httpUsageIteration, len(entries))
	for index, rawEntry := range entries {
		var entry struct {
			Type string `json:"type"`
			httpUsageUpdate
		}
		if err := json.Unmarshal(rawEntry, &entry); err != nil {
			return httpUsageReceipt{}, nil, errors.Wrapf(err, "decode Claude usage iteration %d", index)
		}
		if entry.Type == "" {
			return httpUsageReceipt{}, nil, errors.Errorf("Claude usage iteration %d has no type", index)
		}
		state := httpUsageIteration{kind: entry.Type, updated: entry.httpUsageUpdate}
		if index < len(previous) && previous[index].kind == entry.Type {
			state.receipt = previous[index].receipt
		}
		if err := state.receipt.applyCounters(entry.httpUsageUpdate); err != nil {
			return httpUsageReceipt{}, nil, errors.Wrapf(err, "Claude usage iteration %d", index)
		}
		if state.receipt.input == nil || state.receipt.output == nil {
			return httpUsageReceipt{}, nil, errors.Errorf("Claude usage iteration %d has incomplete token counters", index)
		}
		states[index] = state
	}
	receipts := make([]httpUsageReceipt, len(states))
	for index, state := range states {
		receipts[index] = state.receipt
	}
	total, err := sumHTTPUsageReceipts(receipts)
	if err != nil {
		return httpUsageReceipt{}, nil, err
	}
	return total, states, nil
}

// addHTTPUsageIterationCounter sums a present nonnegative count with overflow checks.
// Parameters: target owns the running total and value is a verified counter.
// Returns: an error without changing target when the canonical sum would overflow.
func addHTTPUsageIterationCounter(target **int, value *int) error {
	if value == nil {
		return nil
	}
	previous := 0
	if *target != nil {
		previous = **target
	}
	if *value < 0 || *value > math.MaxInt-previous {
		return errors.New("Claude usage iteration total overflows")
	}
	sum := previous + *value
	*target = &sum
	return nil
}

// sumHTTPUsageIterationComponent selects compaction or non-compaction receipts.
// Parameters: states are verified indexed receipts and compaction selects the
// component. Returns: exact summed counters or an error without modifying states.
func sumHTTPUsageIterationComponent(states []httpUsageIteration, compaction bool) (httpUsageReceipt, error) {
	var receipts []httpUsageReceipt
	for _, state := range states {
		if (state.kind == "compaction") == compaction {
			receipts = append(receipts, state.receipt)
		}
	}
	return sumHTTPUsageReceipts(receipts)
}

// sumHTTPUsageReceipts adds independent token and cache receipts exactly once.
// Parameters: receipts contain already validated counters and TTL splits.
// Returns: canonical totals, normalizing aggregate-only writes to the existing
// five-minute default, or an error before exposing an overflowing sum.
func sumHTTPUsageReceipts(receipts []httpUsageReceipt) (httpUsageReceipt, error) {
	input, output := 0, 0
	total := httpUsageReceipt{input: &input, output: &output}
	for _, receipt := range receipts {
		for _, pair := range [][2]**int{
			{&total.input, &receipt.input},
			{&total.output, &receipt.output},
			{&total.cached, &receipt.cached},
		} {
			if err := addHTTPUsageIterationCounter(pair[0], *pair[1]); err != nil {
				return httpUsageReceipt{}, err
			}
		}
		write5m, write1h := receipt.write5m, receipt.write1h
		if write5m == nil && write1h == nil {
			write5m = receipt.creation
		}
		if err := addHTTPUsageIterationCounter(&total.write5m, write5m); err != nil {
			return httpUsageReceipt{}, err
		}
		if err := addHTTPUsageIterationCounter(&total.write1h, write1h); err != nil {
			return httpUsageReceipt{}, err
		}
	}
	if err := total.applyCounters(httpUsageUpdate{}); err != nil {
		return httpUsageReceipt{}, err
	}
	return total, nil
}

// latestHTTPUsageIterationBaseline seeds a single message iteration from root evidence.
// Parameters: previous holds indexed receipts and ordinary is the latest verified
// non-compaction aggregate. Returns: an independent slice, mapping root counters
// only when exactly one message component exists; multiple components stay separate.
func latestHTTPUsageIterationBaseline(previous []httpUsageIteration, ordinary httpUsageReceipt) []httpUsageIteration {
	states := append([]httpUsageIteration(nil), previous...)
	messageIndex := -1
	for index, state := range states {
		if state.kind == "compaction" {
			continue
		}
		if messageIndex >= 0 {
			return states
		}
		messageIndex = index
	}
	if messageIndex >= 0 {
		states[messageIndex].receipt = ordinary
	}
	return states
}

// applyHTTPUsageIterationComponent overlays explicitly reported message counters.
// Parameters: ordinary retains cumulative root evidence, states carry current
// indexed field presence and total contains verified indexed sums. Returns: an
// atomic error for contradictions without treating inherited fields as new data.
func applyHTTPUsageIterationComponent(ordinary *httpUsageReceipt, states []httpUsageIteration, total httpUsageReceipt) error {
	update := httpUsageUpdate{}
	for _, state := range states {
		if state.kind == "compaction" {
			continue
		}
		present := state.updated
		if present.Input != nil {
			update.Input = total.input
		}
		if present.Output != nil {
			update.Output = total.output
		}
		if present.Cached != nil {
			update.Cached = total.cached
		}
		if present.Creation != nil {
			creation := 0
			if total.write5m != nil {
				creation += *total.write5m
			}
			if total.write1h != nil {
				creation += *total.write1h
			}
			update.Creation = &creation
		}
		if present.CacheCreation != nil {
			if update.CacheCreation == nil {
				update.CacheCreation = &struct {
					Write5m *int `json:"ephemeral_5m_input_tokens"`
					Write1h *int `json:"ephemeral_1h_input_tokens"`
				}{}
			}
			if present.CacheCreation.Write5m != nil {
				update.CacheCreation.Write5m = total.write5m
			}
			if present.CacheCreation.Write1h != nil {
				update.CacheCreation.Write1h = total.write1h
			}
		}
	}
	next := *ordinary
	// A new explicit TTL split replaces the older aggregate baseline. Omitted
	// splits and repeated aggregate-only updates still retain verified TTLs.
	if update.CacheCreation != nil && update.Creation == nil {
		next.creation = nil
	}
	if err := next.applyCounters(update); err != nil {
		return err
	}
	*ordinary = next
	return nil
}
