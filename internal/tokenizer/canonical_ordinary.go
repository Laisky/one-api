package tiktoken

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// canonicalClPattern and canonicalO2Pattern pin the only patterns eligible for the fast path.
// Custom patterns, including near matches, retain the original regexp2 implementation.
const canonicalClPattern = `(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?\p{L}+|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n]*|\s*[\r\n]+|\s+(?!\S)|\s+`
const canonicalO2Pattern = `[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]*[\p{Ll}\p{Lm}\p{Lo}\p{M}]+(?i:'s|'t|'re|'ve|'m|'ll|'d)?|[^\r\n\p{L}\p{N}]?[\p{Lu}\p{Lt}\p{Lm}\p{Lo}\p{M}]+[\p{Ll}\p{Lm}\p{Lo}\p{M}]*(?i:'s|'t|'re|'ve|'m|'ll|'d)?|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n/]*|\s*[\r\n]+|\s+(?!\S)|\s+`

// canonicalSpace mirrors regexp2's default unicode.IsSpace, not Go regexp's ASCII \s.
const canonicalSpace = `\x{0009}-\x{000D}\x{0020}\x{0085}\p{Z}`
const canonicalYieldBytes = 4 * 1024

var canonicalClRegexp = compileCanonicalOrdinary(canonicalClPattern)
var canonicalO2Regexp = compileCanonicalOrdinary(canonicalO2Pattern)

// compileCanonicalOrdinary compiles pinned alternatives, leaving whitespace lookahead to explicit code.
func compileCanonicalOrdinary(pattern string) *regexp.Regexp {
	pattern = strings.TrimSuffix(pattern, `|\s+(?!\S)|\s+`)
	// regexp2 lowercases runes for IgnoreCase. SimpleFold would additionally match
	// U+017F (long s), so contractions must enumerate ASCII case variants instead.
	pattern = strings.ReplaceAll(pattern, `(?i:'s|'t|'re|'ve|'m|'ll|'d)`, `(?:'[sS]|'[tT]|'[rR][eE]|'[vV][eE]|'[mM]|'[lL][lL]|'[dD])`)
	pattern = strings.ReplaceAll(pattern, `[^\s\p{L}\p{N}]`, `[^`+canonicalSpace+`\p{L}\p{N}]`)
	pattern = strings.ReplaceAll(pattern, `\s*`, `[`+canonicalSpace+`]*`)
	return regexp.MustCompile(`^(?:` + pattern + `)`)
}

// canonicalOrdinaryRegexp selects a shared immutable matcher only for an exact supported core pattern.
func canonicalOrdinaryRegexp(pattern string) *regexp.Regexp {
	switch pattern {
	case canonicalClPattern:
		return canonicalClRegexp
	case canonicalO2Pattern:
		return canonicalO2Regexp
	default:
		return nil
	}
}

// canonicalPieceLength returns the next original-pattern match's byte length in valid UTF-8 text.
// Whitespace before a nonspace backtracks one rune when possible for \s+(?!\S).
func canonicalPieceLength(text string, matcher *regexp.Regexp) int {
	if match := matcher.FindStringIndex(text); match != nil {
		return match[1]
	}
	end, lastStart := 0, 0
	for end < len(text) {
		r, size := utf8.DecodeRuneInString(text[end:])
		if !unicode.IsSpace(r) {
			break
		}
		lastStart, end = end, end+size
	}
	if end < len(text) && lastStart > 0 {
		return lastStart
	}
	return end
}

// encodeCanonicalOrdinary preserves complete token IDs and the unchanged merge kernel.
// The scheduling callback runs synchronously after a 4 KiB budget and only when
// input remains; one large BPE piece is not subdivided or given a latency bound.
func (bp *CoreBPE) encodeCanonicalOrdinary(text string, yield func()) []int {
	if !utf8.ValidString(text) {
		// Match the upstream []rune conversion, including one replacement per invalid byte.
		text = string([]rune(text))
	}
	ret := []int{}
	budget := 0
	for len(text) > 0 {
		n := canonicalPieceLength(text, bp.canonicalOrdinary)
		if n == 0 {
			// Defensive compatibility fallback. Canonical patterns cover every rune,
			// but an unexpected unmatched input must retain upstream behavior.
			for _, match := range findRegex2AllStringMatchIndex(text, bp.tlRegex) {
				piece := cutRunes([]rune(text), match[0], match[1])
				if token, ok := bp.encoder[piece]; ok {
					ret = append(ret, token)
				} else {
					ret = append(ret, bytePairEncode([]byte(piece), bp.encoder)...)
				}
			}
			return ret
		}
		piece := text[:n]
		if token, ok := bp.encoder[piece]; ok {
			ret = append(ret, token)
		} else {
			ret = append(ret, bytePairEncode([]byte(piece), bp.encoder)...)
		}
		text = text[n:]
		budget += n
		if budget >= canonicalYieldBytes && len(text) != 0 {
			yield()
			budget = 0
		}
	}
	return ret
}
