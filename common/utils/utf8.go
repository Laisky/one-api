package utils

import (
	"strings"
	"unicode/utf8"
)

// ToValidUTF8 returns s with every run of invalid UTF-8 bytes replaced by
// U+FFFD. Valid strings are returned unchanged without allocating.
//
// Why: request paths, query strings, header values and upstream payloads are
// byte strings, while Prometheus label values, OTLP protobuf string fields and
// MySQL/PostgreSQL text columns all require valid UTF-8 and either panic,
// reject the whole batch, or fail the INSERT otherwise.
// Parameters: s is the candidate string. Return value: a valid UTF-8 string.
func ToValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "�")
}

// TruncateUTF8 returns the longest prefix of s that is at most maxBytes long
// and does not end in the middle of a multi-byte rune, so truncating a valid
// string keeps it valid. Strings within the budget are returned unchanged.
// Parameters: s is the string to bound; maxBytes is the byte budget.
// Return value: the bounded prefix.
func TruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	if maxBytes <= 0 {
		return ""
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
