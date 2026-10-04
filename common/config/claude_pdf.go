package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// DefaultClaudeNativePDFTokensPerKiB is a tentative calibration for review,
// equivalent to one estimated token per sixteen decoded bytes. It is neither
// an empirical PDF token measurement nor a guaranteed funding upper bound.
const DefaultClaudeNativePDFTokensPerKiB = 64

// ClaudeNativePDFTokensPerKiB is the trusted startup rate for decoded native PDF
// bytes. Unknown-size sources use the separate document fallback allowance.
var ClaudeNativePDFTokensPerKiB = loadClaudeNativePDFTokensPerKiB()

// loadClaudeNativePDFTokensPerKiB rejects invalid explicit startup rates instead
// of silently disabling or clamping the linear document estimate.
func loadClaudeNativePDFTokensPerKiB() int {
	raw, present := os.LookupEnv("CLAUDE_NATIVE_PDF_TOKENS_PER_KIB")
	value, err := parseClaudeNativePDFTokensPerKiB(raw, present)
	if err != nil {
		panic(err)
	}
	return value
}

// parseClaudeNativePDFTokensPerKiB returns the tentative default when unset and
// otherwise validates a positive representable integer tokens-per-KiB rate.
func parseClaudeNativePDFTokensPerKiB(raw string, present bool) (int, error) {
	if !present {
		return DefaultClaudeNativePDFTokensPerKiB, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return 0, errors.New("CLAUDE_NATIVE_PDF_TOKENS_PER_KIB must be a positive representable integer")
	}
	return value, nil
}
