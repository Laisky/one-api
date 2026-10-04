package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// DefaultClaudeNativeDocumentTokenAllowance is a prepaid estimate for one opaque
// native document, not a provider price or guaranteed document/context maximum.
const DefaultClaudeNativeDocumentTokenAllowance = 32768

// ClaudeNativeDocumentTokenAllowance is a trusted startup setting. Every native
// unknown-size file/URL or non-PDF document reserves this many input-token units in addition to
// its textual metadata. Complete provider receipts reconcile the estimate.
var ClaudeNativeDocumentTokenAllowance = loadClaudeNativeDocumentTokenAllowance()

// loadClaudeNativeDocumentTokenAllowance fails startup on explicitly invalid
// settings instead of silently accepting a zero or overflowing reservation.
func loadClaudeNativeDocumentTokenAllowance() int {
	raw, present := os.LookupEnv("CLAUDE_NATIVE_DOCUMENT_TOKEN_ALLOWANCE")
	value, err := parseClaudeNativeDocumentTokenAllowance(raw, present)
	if err != nil {
		panic(err)
	}
	return value
}

// parseClaudeNativeDocumentTokenAllowance validates the operator's estimate;
// caller-supplied page counts or token hints cannot change this setting.
func parseClaudeNativeDocumentTokenAllowance(raw string, present bool) (int, error) {
	if !present {
		return DefaultClaudeNativeDocumentTokenAllowance, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > 1048576 {
		return 0, errors.New("CLAUDE_NATIVE_DOCUMENT_TOKEN_ALLOWANCE must be an integer in 1..1048576")
	}
	return value, nil
}
