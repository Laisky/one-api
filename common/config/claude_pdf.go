package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// DefaultClaudeNativePDFTokensPerPage is the admission estimate for one native
// PDF page, derived from Anthropic's published PDF and vision costs (verified
// 2026-10): each page is billed as extracted text plus a rasterized page image.
//
//   - Text: "Each page typically uses 1,500-3,000 tokens per page depending on
//     content density" (https://platform.claude.com/docs/en/build-with-claude/pdf-support).
//     The Claude 4.7+ tokenizer produces about 30 percent more tokens
//     (https://platform.claude.com/docs/en/build-with-claude/token-counting),
//     so the upper typical text cost is 3,000 x 1.3 = 3,900.
//   - Image: the high-resolution tier caps one image at 4,784 tokens; the
//     standard tier caps it at 1,568
//     (https://platform.claude.com/docs/en/build-with-claude/vision).
//
// The default 3,900 + 4,784 = 8,684 covers a typical dense page on every
// current model tier. It is a prepayment estimate; complete provider receipts
// remain authoritative at settlement.
const DefaultClaudeNativePDFTokensPerPage = 8684

// maxClaudeNativePDFTokensPerPage bounds the operator setting so the per-request
// product of pages and tokens per page stays far inside the integer range.
const maxClaudeNativePDFTokensPerPage = 1048576

// ClaudeNativePDFTokensPerPage is the trusted startup estimate per native PDF
// page. Request-supplied page counts or token hints never change it.
var ClaudeNativePDFTokensPerPage = loadClaudeNativePDFTokensPerPage()

// loadClaudeNativePDFTokensPerPage reads CLAUDE_NATIVE_PDF_TOKENS_PER_PAGE and
// panics at startup on an explicitly invalid value instead of silently falling
// back. It returns the validated per-page estimate.
func loadClaudeNativePDFTokensPerPage() int {
	raw, present := os.LookupEnv("CLAUDE_NATIVE_PDF_TOKENS_PER_PAGE")
	value, err := parseClaudeNativePDFTokensPerPage(raw, present)
	if err != nil {
		panic(err)
	}
	return value
}

// parseClaudeNativePDFTokensPerPage returns the documented default when the
// variable is unset; otherwise raw must be an integer in 1..1048576. It returns
// the parsed value or a validation error.
func parseClaudeNativePDFTokensPerPage(raw string, present bool) (int, error) {
	if !present {
		return DefaultClaudeNativePDFTokensPerPage, nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 || value > maxClaudeNativePDFTokensPerPage {
		return 0, errors.New("CLAUDE_NATIVE_PDF_TOKENS_PER_PAGE must be an integer in 1..1048576")
	}
	return value, nil
}
