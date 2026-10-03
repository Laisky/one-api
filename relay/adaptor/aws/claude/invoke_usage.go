package aws

import (
	"bytes"
	"encoding/json"
	"math"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/model"
)

// invokeUsageReceipt tracks presence as well as values. A missing receipt is
// not an explicit zero-cost receipt; omitted fields in cumulative deltas must
// not erase earlier counts. input_tokens excludes cache buckets on Bedrock.
type invokeUsageReceipt struct {
	input    *int
	output   *int
	cached   *int
	creation *int
	write5m  *int
	write1h  *int
}

type invokeUsageUpdate struct {
	Input         *int `json:"input_tokens"`
	Output        *int `json:"output_tokens"`
	Cached        *int `json:"cache_read_input_tokens"`
	Creation      *int `json:"cache_creation_input_tokens"`
	CacheCreation *struct {
		Write5m *int `json:"ephemeral_5m_input_tokens"`
		Write1h *int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// apply validates and atomically merges an upstream cumulative usage object.
// Invalid, negative, or overflowing counters leave the previous receipt intact.
func (r *invokeUsageReceipt) apply(raw json.RawMessage) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var update invokeUsageUpdate
	if err := json.Unmarshal(raw, &update); err != nil {
		return errors.Wrap(err, "decode Claude usage receipt")
	}
	next := *r
	pairs := [][2]**int{{&next.input, &update.Input}, {&next.output, &update.Output}, {&next.cached, &update.Cached}, {&next.creation, &update.Creation}}
	if update.CacheCreation != nil {
		pairs = append(pairs, [2]**int{&next.write5m, &update.CacheCreation.Write5m}, [2]**int{&next.write1h, &update.CacheCreation.Write1h})
	}
	for _, pair := range pairs {
		value := *pair[1]
		if value == nil {
			continue
		}
		if *value < 0 {
			return errors.New("Claude usage contains a negative token count")
		}
		*pair[0] = value
	}
	// A repeated aggregate does not erase a previously reported TTL split.
	// Otherwise a final total-only delta could silently rebill one-hour writes
	// at the cheaper five-minute price.
	sum := 0
	values := []*int{next.input, next.output, next.cached}
	if next.write5m != nil || next.write1h != nil {
		values = append(values, next.write5m, next.write1h)
	} else {
		values = append(values, next.creation)
	}
	for _, v := range values {
		if v == nil {
			continue
		}
		if *v > math.MaxInt-sum {
			return errors.New("Claude usage token total overflows")
		}
		sum += *v
	}
	if next.creation != nil && (next.write5m != nil || next.write1h != nil) {
		split := 0
		if next.write5m != nil {
			split += *next.write5m
		}
		if next.write1h != nil {
			split += *next.write1h
		}
		if split != *next.creation {
			return errors.New("Claude cache creation total contradicts its TTL split")
		}
	}
	*r = next
	return nil
}

// snapshot returns the last verified token receipt, or nil when no counters
// were reported. It returns a fresh value so clients cannot mutate the state.
func (r *invokeUsageReceipt) snapshot() *model.Usage {
	if r.input == nil && r.output == nil && r.cached == nil && r.creation == nil && r.write5m == nil && r.write1h == nil {
		return nil
	}
	usage := &model.Usage{}
	if r.input != nil {
		usage.PromptTokens = *r.input
	}
	if r.output != nil {
		usage.CompletionTokens = *r.output
	}
	if r.cached != nil && *r.cached > 0 {
		usage.PromptTokensDetails = &model.UsagePromptTokensDetails{CachedTokens: *r.cached}
	}
	if r.write5m != nil || r.write1h != nil {
		if r.write5m != nil {
			usage.CacheWrite5mTokens = *r.write5m
		}
		if r.write1h != nil {
			usage.CacheWrite1hTokens = *r.write1h
		}
	} else if r.creation != nil {
		usage.CacheWrite5mTokens = *r.creation
	}
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage
}
