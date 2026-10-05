package realtime

import (
	"math"

	"github.com/Laisky/errors/v2"
)

// ErrBudgetExhausted reports that a session's prepaid reservation cannot fund
// more provider work. Transports must stop forwarding when they receive it.
var ErrBudgetExhausted = errors.New("realtime session budget exhausted")

// ErrUnpriceableInput reports client work whose cost cannot be bounded before
// forwarding, such as a provider-fetched file reference or an unknown media type.
var ErrUnpriceableInput = errors.New("realtime input cannot be priced before forwarding")

// Estimate is a conservative token-level upper bound of session work that has
// no authoritative receipt yet. Modality-specific fields hold work whose kind is
// known from the wire; Context holds re-billed conversation input and Output
// holds output allowances whose modality is unknown, so a SpendGate prices them
// at the session's most expensive input and output rates respectively.
type Estimate struct {
	Text, Audio, Image, Video int64
	OutputText, OutputAudio   int64
	Context, Output           int64
}

// Add returns the saturating sum of e and other. Parameters: other is a second
// nonnegative estimate. Returns: the combined estimate; values never wrap.
func (e Estimate) Add(other Estimate) Estimate {
	return Estimate{
		Text: saturatingAdd(e.Text, other.Text), Audio: saturatingAdd(e.Audio, other.Audio),
		Image: saturatingAdd(e.Image, other.Image), Video: saturatingAdd(e.Video, other.Video),
		OutputText:  saturatingAdd(e.OutputText, other.OutputText),
		OutputAudio: saturatingAdd(e.OutputAudio, other.OutputAudio),
		Context:     saturatingAdd(e.Context, other.Context), Output: saturatingAdd(e.Output, other.Output),
	}
}

// InputTokens returns every input token of e, including context. Parameters:
// none. Returns: the saturating total.
func (e Estimate) InputTokens() int64 {
	return saturatingAdd(saturatingAdd(saturatingAdd(e.Text, e.Audio), saturatingAdd(e.Image, e.Video)), e.Context)
}

// OutputTokens returns every output token of e, including allowances.
// Parameters: none. Returns: the saturating total.
func (e Estimate) OutputTokens() int64 {
	return saturatingAdd(saturatingAdd(e.OutputText, e.OutputAudio), e.Output)
}

// TotalTokens returns every input and output token of e. Parameters: none.
// Returns: the saturating total.
func (e Estimate) TotalTokens() int64 { return saturatingAdd(e.InputTokens(), e.OutputTokens()) }

// IsZero reports whether e describes no work. Parameters: none. Returns: true
// when every bucket is zero.
func (e Estimate) IsZero() bool { return e == Estimate{} }

// saturatingAdd adds two nonnegative counts without integer wraparound.
// Parameters: a and b are counts. Returns: their sum, or MaxInt64 on overflow.
func saturatingAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// SpendGate funds provider work before a transport forwards it and keeps the
// session's prepaid reservation ahead of observed and estimated spend. All
// methods must be safe for concurrent use by a session's two socket readers.
type SpendGate interface {
	// Commit adds one authoritative receipt to observed spend. Parameters:
	// record is a decoded provider receipt. Returns: none; pricing gaps remain
	// visible to settlement, which reprices the complete ledger.
	Commit(record Record)
	// Ensure atomically reserves enough durable quota for observed spend plus
	// pending, the bound of all unreceipted work including any work about to be
	// forwarded. Parameters: pending is that bound. Returns: nil when funded,
	// or an error wrapping ErrBudgetExhausted; the caller must not forward.
	Ensure(pending Estimate) error
	// Finish records the unreceipted work the provider demonstrably received
	// once both readers have joined. Parameters: evidence excludes allowances
	// for turns that never started. Returns: none.
	Finish(evidence Estimate)
}
