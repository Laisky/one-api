// Package realtime accounts for authoritative Realtime server usage, independently
// of WebSocket transport, provider pricing, and database settlement.
package realtime

import (
	"math"

	"github.com/Laisky/errors/v2"
)

// Tokens contains inclusive input/output counts. Cached tokens are subsets of
// their respective input modalities, never additional input tokens.
type Tokens struct {
	Input             int64 `json:"input_tokens"`
	Output            int64 `json:"output_tokens"`
	Text              int64 `json:"text_tokens"`
	Audio             int64 `json:"audio_tokens"`
	Image             int64 `json:"image_tokens"`
	CachedText        int64 `json:"cached_text_tokens"`
	CachedAudio       int64 `json:"cached_audio_tokens"`
	CachedImage       int64 `json:"cached_image_tokens"`
	CachedUnallocated int64 `json:"cached_unallocated_tokens,omitempty"`
	OutputText        int64 `json:"output_text_tokens"`
	OutputAudio       int64 `json:"output_audio_tokens"`
}

// Record describes one final response or one independent input transcription.
// Model is empty for the channel-bound conversation model. Seconds is used only
// for authoritative duration usage; transcript length is never a duration proxy.
type Record struct {
	Model    string  `json:"model,omitempty"`
	Tokens   Tokens  `json:"tokens"`
	Seconds  float64 `json:"seconds,omitempty"`
	Duration bool    `json:"duration,omitempty"`
}

// Rates contains final, ungrouped quota-per-token rates, plus quota per second.
// All modality rates are absolute, not completion or cache multipliers.
type Rates struct {
	Text, Audio, Image                   float64
	CachedText, CachedAudio, CachedImage float64
	OutputText, OutputAudio, Second      float64
}

// Cost returns the unrounded cost of record using rates. For unallocated cache
// it returns the minimum cost consistent with the reported totals, not a guessed
// exact split. The original record stays unchanged for reconciliation. Invalid
// prices and overflowing results are rejected, never converted to negative debits.
func Cost(record Record, rates Rates) (float64, error) {
	values := []float64{rates.Text, rates.Audio, rates.Image, rates.CachedText,
		rates.CachedAudio, rates.CachedImage, rates.OutputText, rates.OutputAudio, rates.Second}
	for _, value := range values {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, errors.WithStack(ErrInvalidPrice)
		}
	}
	t := record.Tokens
	var cost float64
	if record.Duration {
		if record.Seconds < 0 || math.IsNaN(record.Seconds) || math.IsInf(record.Seconds, 0) {
			return 0, errors.Wrap(ErrInvalidUsage, "invalid realtime duration")
		}
		cost = record.Seconds * rates.Second
	} else {
		if err := t.Validate(); err != nil {
			return 0, err
		}
		t = minimumCostCacheAllocation(t, rates)
		cost = float64(t.Text-t.CachedText)*rates.Text + float64(t.CachedText)*rates.CachedText +
			float64(t.Audio-t.CachedAudio)*rates.Audio + float64(t.CachedAudio)*rates.CachedAudio +
			float64(t.Image-t.CachedImage)*rates.Image + float64(t.CachedImage)*rates.CachedImage +
			float64(t.OutputText)*rates.OutputText + float64(t.OutputAudio)*rates.OutputAudio
	}
	if math.IsNaN(cost) || math.IsInf(cost, 0) || cost >= float64(math.MaxInt64) {
		return 0, errors.Wrap(ErrQuotaOverflow, "realtime cost exceeds quota range")
	}
	return cost, nil
}

// minimumCostCacheAllocation allocates only the unknown subset of validated t
// to available modalities in descending discount order. This solves the bounded
// three-bucket linear minimum without inventing a provider-reported cache split.
func minimumCostCacheAllocation(t Tokens, rates Rates) Tokens {
	remaining := t.CachedUnallocated
	counts := [3]int64{t.Text, t.Audio, t.Image}
	cached := [3]int64{t.CachedText, t.CachedAudio, t.CachedImage}
	savings := [3]float64{rates.Text - rates.CachedText, rates.Audio - rates.CachedAudio, rates.Image - rates.CachedImage}
	for remaining > 0 {
		best := -1
		for i := range counts {
			if counts[i] > cached[i] && (best < 0 || savings[i] > savings[best]) {
				best = i
			}
		}
		// Validate guarantees enough capacity, including when a cache rate is
		// higher than its ordinary rate; all reported cached tokens still count.
		n := min(remaining, counts[best]-cached[best])
		cached[best] += n
		remaining -= n
	}
	t.CachedText, t.CachedAudio, t.CachedImage = cached[0], cached[1], cached[2]
	t.CachedUnallocated = 0
	return t
}

// Validate verifies the disjoint token partitions of t and returns an error for
// negative counts, overflow, or a cached subset exceeding its parent modality.
func (t Tokens) Validate() error {
	values := []int64{t.Input, t.Output, t.Text, t.Audio, t.Image, t.CachedText,
		t.CachedAudio, t.CachedImage, t.CachedUnallocated, t.OutputText, t.OutputAudio}
	for _, value := range values {
		if value < 0 {
			return errors.Wrap(ErrInvalidUsage, "negative realtime token count")
		}
	}
	if t.Text > t.Input || t.Audio > t.Input-t.Text || t.Image != t.Input-t.Text-t.Audio {
		return errors.Wrap(ErrInvalidUsage, "inconsistent realtime input modalities")
	}
	if t.OutputText > t.Output || t.OutputAudio != t.Output-t.OutputText {
		return errors.Wrap(ErrInvalidUsage, "inconsistent realtime output modalities")
	}
	if t.CachedText > t.Text || t.CachedAudio > t.Audio || t.CachedImage > t.Image ||
		t.CachedUnallocated > t.Input-t.CachedText-t.CachedAudio-t.CachedImage {
		return errors.Wrap(ErrInvalidUsage, "realtime cache exceeds parent modality")
	}
	if t.Input > math.MaxInt64-t.Output {
		return errors.Wrap(ErrQuotaOverflow, "realtime token count overflow")
	}
	return nil
}

// RoundQuota applies the group multiplier once, rounds a session's total once,
// and adds already-grouped tool quota once. Floating arithmetic within four ULPs
// of an integer is normalized so an exact monetary boundary does not gain a
// spurious quota unit. Results outside exact float integer precision are rejected.
func RoundQuota(cost, group float64, tools int64) (int64, error) {
	if cost < 0 || group < 0 || tools < 0 || math.IsNaN(cost) || math.IsNaN(group) || math.IsInf(cost, 0) || math.IsInf(group, 0) {
		return 0, errors.Wrap(ErrInvalidUsage, "invalid realtime settlement amount")
	}
	value := cost * group
	if math.IsNaN(value) || math.IsInf(value, 0) || value >= 1<<53 {
		return 0, errors.Wrap(ErrQuotaOverflow, "realtime settlement exceeds exact quota range")
	}
	nearest := math.Round(value)
	tolerance := 4 * (math.Nextafter(value, math.Inf(1)) - value)
	if nearest > 0 && math.Abs(value-nearest) <= tolerance {
		value = nearest
	}
	rounded := int64(math.Ceil(value))
	if rounded > math.MaxInt64-tools {
		return 0, errors.Wrap(ErrQuotaOverflow, "realtime tool quota overflow")
	}
	return rounded + tools, nil
}
