package realtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// Per-session accounting limits bound every retained collection and its keys.
// Reaching a limit closes the metered transport; it never starts free unmetered
// continuation or evicts deduplication evidence from a still-running session.
const (
	MaxRecords          = 4096
	MaxItems            = 4096
	MaxPendingResponses = 1024
	MaxIdentifierBytes  = 256
	MaxModelBytes       = 256
	MaxIssues           = 16
	maxDiagnosticBytes  = 256
	maxAuditSamples     = 8
)

// stop seals the ledger and returns the transport's terminal capacity signal.
// Capacity by itself is not missing usage; pending work is handled by Finish.
func (l *Ledger) stop() error {
	l.stopped = true
	return errors.WithStack(ErrLedgerLimit)
}

// trackResponse remembers one unresolved response key, without evicting an older
// key that might still deliver a billable terminal receipt.
func (l *Ledger) trackResponse(key string) error {
	l.pendingResponses[key] = struct{}{}
	if len(l.pendingResponses) >= MaxPendingResponses {
		return l.stop()
	}
	return nil
}

// boundedDiagnostic limits retained diagnostic text. JSON encoding safely
// replaces an incomplete final UTF-8 sequence if a byte boundary splits one.
func boundedDiagnostic(message string) string {
	if len(message) > maxDiagnosticBytes {
		return message[:maxDiagnosticBytes]
	}
	return message
}

// AuditSnapshot is bounded log evidence, not a replacement for all live receipts.
// The sample is explicitly labeled; the digest covers the entire ordered set.
// Input/output counters and receipt count always describe the complete ledger.
type AuditSnapshot struct {
	ReceiptCount    int      `json:"receipt_count"`
	InputTokens     int64    `json:"input_tokens"`
	OutputTokens    int64    `json:"output_tokens"`
	SampleRecords   []Record `json:"sample_records,omitempty"`
	SampleTruncated bool     `json:"sample_truncated"`
	RecordsSHA256   string   `json:"records_sha256,omitempty"`
	Issues          []string `json:"issues,omitempty"`
	UsageGap        bool     `json:"usage_gap"`
	CapacityReached bool     `json:"capacity_reached"`
}

// Audit returns an immutable, bounded snapshot for persistent log metadata. All
// receipts remain in the live ledger for pricing; only log serialization samples.
func (l *Ledger) Audit() AuditSnapshot {
	result := AuditSnapshot{ReceiptCount: len(l.Records), InputTokens: l.InputTokens,
		OutputTokens: l.OutputTokens, SampleTruncated: len(l.Records) > maxAuditSamples,
		UsageGap: l.HasUsageGap(), CapacityReached: l.stopped}
	result.SampleRecords = append([]Record(nil), l.Records[:min(len(l.Records), maxAuditSamples)]...)
	for i := range result.SampleRecords {
		if len(result.SampleRecords[i].Model) > MaxModelBytes {
			result.SampleRecords[i].Model = result.SampleRecords[i].Model[:MaxModelBytes]
		}
	}
	for _, issue := range l.Issues[:min(len(l.Issues), MaxIssues)] {
		result.Issues = append(result.Issues, boundedDiagnostic(issue))
	}
	if data, err := json.Marshal(l.Records); err == nil {
		digest := sha256.Sum256(data)
		result.RecordsSHA256 = hex.EncodeToString(digest[:])
	}
	return result
}
