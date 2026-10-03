package realtime

import "github.com/Laisky/errors/v2"

// Sentinel errors let transport, settlement and tests distinguish recoverable
// pricing ambiguity from missing evidence, invalid data and a required stop.
var (
	ErrInvalidUsage    = errors.New("invalid realtime usage")
	ErrInvalidPrice    = errors.New("invalid realtime price")
	ErrQuotaOverflow   = errors.New("realtime quota overflow")
	ErrIncompleteUsage = errors.New("incomplete realtime usage")
	ErrAmbiguousCache  = errors.New("mixed realtime cache lacks modality split")
	ErrLedgerLimit     = errors.New("realtime billing ledger capacity reached")
)
