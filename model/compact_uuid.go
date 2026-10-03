package model

// This file implements the shared compact UUID codec (AUTO-002): source normalization, the
// RFC/network-order byte representation, and the driver scan/bind behavior used by compact
// projections and by the migration coordinator.
//
// The codec never rewrites authoritative legacy text. It only derives the additive shadow
// representation, and it derives it identically to every dialect's synchronization trigger —
// the parity vectors in the tests hold the Go implementation and the generated trigger SQL to
// the same accept/reject boundary.
//
// No error returned from this file contains a UUID value: a malformed identifier is often
// attacker-supplied request input, and the proposal forbids leaking identifier values into
// logs and error text.

import (
	"database/sql/driver"
	"strings"

	"github.com/Laisky/errors/v2"
)

// compactUUIDLen is the byte length of the RFC/network-order representation.
const compactUUIDLen = 16

// compactUUIDTextLen is the exact accepted length of a canonical hyphenated UUID.
const compactUUIDTextLen = 36

// compactUUIDGoldenText is the proposal's fixed golden canonical vector.
const compactUUIDGoldenText = "018f0000-0000-7000-8000-000000000001"

// compactUUIDGoldenHex is the proposal's fixed golden byte vector, hex-encoded.
const compactUUIDGoldenHex = "018f0000000070008000000000000001"

// compactUUIDHyphenPositions are the only accepted hyphen offsets in the 8-4-4-4-12 grammar.
var compactUUIDHyphenPositions = [4]int{8, 13, 18, 23}

// compactUUIDVersionPosition is the offset of the version nibble ('7' for UUIDv7).
const compactUUIDVersionPosition = 14

// compactUUIDVariantPosition is the offset of the variant nibble (RFC variant '8', '9', 'a', 'b').
const compactUUIDVariantPosition = 19

// compactUUID is the RFC/network-order 16-byte representation of an external UUID.
//
// The byte order is exactly the canonical text's field order with hyphens removed. No
// timestamp-field swapping is performed: MySQL's UUID_TO_BIN swap flag is forbidden by the
// proposal, so every dialect and the Go codec agree byte-for-byte.
type compactUUID [compactUUIDLen]byte

// errCompactUUIDInvalid is the sentinel for any value the codec rejects.
//
// It is deliberately valueless. Callers that must distinguish a rejection from a database
// failure match this sentinel; nothing derived from the offending input reaches the message.
var errCompactUUIDInvalid = errors.New("value is not a canonical external uuid")

// parseCompactUUID converts canonical UUID text into RFC-order bytes under the strict
// source-normalization contract.
//
// Accepted input is exactly 36 ASCII characters in case-insensitive 8-4-4-4-12 hexadecimal
// form, with no surrounding whitespace, UUID version 7, and RFC variant [89ab]. Anything else
// is rejected. Trimming is the caller's job and happens at the request boundary, not here:
// the trigger contract compares the stored text byte-for-byte, so a codec that silently
// trimmed would accept values its SQL counterpart derives to NULL.
//
// Parameters:
//   - text: candidate canonical UUID text.
//
// Return values:
//   - compactUUID: RFC-order bytes for accepted input.
//   - error: errCompactUUIDInvalid-wrapped error carrying no UUID value.
func parseCompactUUID(text string) (compactUUID, error) {
	var value compactUUID
	if len(text) != compactUUIDTextLen {
		return value, errors.Wrap(errCompactUUIDInvalid, "length is not 36 characters")
	}
	for _, position := range compactUUIDHyphenPositions {
		if text[position] != '-' {
			return value, errors.Wrap(errCompactUUIDInvalid, "hyphens are misplaced")
		}
	}

	written := 0
	for position := 0; position < compactUUIDTextLen; position++ {
		if position == compactUUIDHyphenPositions[0] || position == compactUUIDHyphenPositions[1] ||
			position == compactUUIDHyphenPositions[2] || position == compactUUIDHyphenPositions[3] {
			continue
		}
		high, ok := compactHexNibble(text[position])
		if !ok {
			return value, errors.Wrap(errCompactUUIDInvalid, "value contains a non-hexadecimal character")
		}
		position++
		low, ok := compactHexNibble(text[position])
		if !ok {
			return value, errors.Wrap(errCompactUUIDInvalid, "value contains a non-hexadecimal character")
		}
		value[written] = high<<4 | low
		written++
	}
	if written != compactUUIDLen {
		return compactUUID{}, errors.Wrap(errCompactUUIDInvalid, "value does not decode to 16 bytes")
	}

	if lowerASCII(text[compactUUIDVersionPosition]) != '7' {
		return compactUUID{}, errors.Wrap(errCompactUUIDInvalid, "uuid version is not 7")
	}
	switch lowerASCII(text[compactUUIDVariantPosition]) {
	case '8', '9', 'a', 'b':
	default:
		return compactUUID{}, errors.Wrap(errCompactUUIDInvalid, "uuid variant is not the rfc variant")
	}
	return value, nil
}

// compactHexNibble decodes one ASCII hexadecimal character, accepting either case.
// Parameters:
//   - char: candidate character.
//
// Return values:
//   - byte: decoded 4-bit value.
//   - bool: true when the character is hexadecimal.
func compactHexNibble(char byte) (byte, bool) {
	switch {
	case char >= '0' && char <= '9':
		return char - '0', true
	case char >= 'a' && char <= 'f':
		return char - 'a' + 10, true
	case char >= 'A' && char <= 'F':
		return char - 'A' + 10, true
	default:
		return 0, false
	}
}

// lowerASCII lowercases one ASCII character without allocating.
// Parameters:
//   - char: candidate character.
//
// Return values:
//   - byte: lowercase character for A-Z, otherwise the input unchanged.
func lowerASCII(char byte) byte {
	if char >= 'A' && char <= 'Z' {
		return char + ('a' - 'A')
	}
	return char
}

// compactHexDigits is the lowercase alphabet used to format canonical text.
const compactHexDigits = "0123456789abcdef"

// canonical formats the bytes as lowercase canonical hyphenated UUID text.
// Accepted input is normalized to lowercase, so a mixed-case legacy value and its lowercase
// equivalent produce the same canonical comparison string.
// Parameters: none.
//
// Return values:
//   - string: lowercase 8-4-4-4-12 representation.
func (value compactUUID) canonical() string {
	out := make([]byte, 0, compactUUIDTextLen)
	for index, b := range value {
		switch index {
		case 4, 6, 8, 10:
			out = append(out, '-')
		}
		out = append(out, compactHexDigits[b>>4], compactHexDigits[b&0x0f])
	}
	return string(out)
}

// bytes returns a fresh copy of the RFC-order representation.
//
// The copy is required, not defensive habit: the slice is handed to a driver as a bind value
// and may be retained past the statement, and a slice aliasing the array of a value that the
// caller later reuses would corrupt a derived shadow.
// Parameters: none.
//
// Return values:
//   - []byte: newly allocated 16-byte slice.
func (value compactUUID) bytes() []byte {
	out := make([]byte, compactUUIDLen)
	copy(out, value[:])
	return out
}

// nullCompactUUID scans and binds a compact shadow value while distinguishing SQL NULL from
// an all-zero 16-byte value.
//
// The distinction is load-bearing. A nullable FK's compact shadow is NULL, and the all-zero
// UUID is a value the codec rejects as non-v7 — conflating the two would let a zero-filled
// shadow read as "not yet derived" and vice versa.
type nullCompactUUID struct {
	// value holds the RFC-order bytes when valid is true.
	value compactUUID
	// valid reports whether the database column was non-NULL.
	valid bool
}

// Scan implements sql.Scanner for every supported dialect's compact representation.
//
// PostgreSQL's native uuid arrives as canonical text or as raw bytes depending on how the
// driver renders it for database/sql; MySQL BINARY(16) and SQLite BLOB arrive as raw bytes.
// The two byte forms are unambiguous by length: 16 is the raw representation and 36 is ASCII
// text. Every accepted byte slice is copied because the driver may reuse its buffer after the
// next row is scanned.
// Parameters:
//   - src: driver value for the compact column.
//
// Return values:
//   - error: wrapped error when the value has an unexpected type, length, or grammar.
func (null *nullCompactUUID) Scan(src any) error {
	*null = nullCompactUUID{}
	switch typed := src.(type) {
	case nil:
		return nil
	case [compactUUIDLen]byte:
		null.value = compactUUID(typed)
		null.valid = true
		return nil
	case []byte:
		return null.scanBytes(typed)
	case string:
		return null.scanText(typed)
	default:
		return errors.Errorf("compact uuid column has unsupported driver type %T", src)
	}
}

// scanBytes decodes a raw 16-byte representation or a 36-byte ASCII rendering.
// Parameters:
//   - src: driver byte slice, which the driver may reuse after this call.
//
// Return values:
//   - error: wrapped error when the length is neither 16 nor 36, or the text is malformed.
func (null *nullCompactUUID) scanBytes(src []byte) error {
	switch len(src) {
	case compactUUIDLen:
		// Copy before retaining: the driver owns this buffer only until the next scan.
		copy(null.value[:], src)
		null.valid = true
		return nil
	case compactUUIDTextLen:
		return null.scanText(string(src))
	default:
		return errors.Errorf("compact uuid column has unexpected byte length %d", len(src))
	}
}

// scanText decodes a canonical text rendering of a compact column.
// Parameters:
//   - src: canonical UUID text.
//
// Return values:
//   - error: wrapped error when the text is not a canonical UUIDv7.
func (null *nullCompactUUID) scanText(src string) error {
	value, err := parseCompactUUID(src)
	if err != nil {
		return errors.Wrap(err, "decode compact uuid column text")
	}
	null.value = value
	null.valid = true
	return nil
}

// Value implements driver.Valuer for dialects that accept the raw byte representation.
//
// It is the MySQL/SQLite binding. PostgreSQL binds through compactBindValue instead, because
// only the caller knows the dialect and PostgreSQL's native uuid parameter must be sent as
// canonical text rather than as a 16-byte value.
// Parameters: none.
//
// Return values:
//   - driver.Value: a fresh 16-byte slice, or nil for a NULL shadow.
//   - error: always nil; the signature satisfies driver.Valuer.
func (null nullCompactUUID) Value() (driver.Value, error) {
	if !null.valid {
		return nil, nil
	}
	return null.value.bytes(), nil
}

// compactBindValue returns the bind value for one compact column on a given dialect.
//
// The value is bound directly, with no column-side cast or conversion function: a predicate
// such as `WHERE uuid_compact = ?` must remain sargable against the compact index, and
// wrapping the column in a cast would silently disqualify it.
//
// PostgreSQL receives canonical text, which the server accepts for a native uuid parameter
// once it has described $n from the column's type. MySQL BINARY(16) and SQLite BLOB receive a
// freshly copied 16-byte slice.
// Parameters:
//   - dialect: lowercase dialect name from dialectName.
//   - value: RFC-order compact value to bind.
//
// Return values:
//   - any: dialect-appropriate bind value.
func compactBindValue(dialect string, value compactUUID) any {
	if dialect == "postgres" {
		return value.canonical()
	}
	return value.bytes()
}

// compactNullBindValue returns the bind value for a possibly-NULL compact column.
// Parameters:
//   - dialect: lowercase dialect name from dialectName.
//   - value: shadow value to bind.
//
// Return values:
//   - any: dialect-appropriate bind value, or nil for a NULL shadow.
func compactNullBindValue(dialect string, value nullCompactUUID) any {
	if !value.valid {
		return nil
	}
	return compactBindValue(dialect, value.value)
}

// deriveCompactFromLegacy applies the registry's derivation rule to one observed legacy value.
//
// This is the single Go definition of the derivation table in the proposal, and it is the
// exact rule every dialect's trigger implements:
//
//   - a valid UUIDv7 derives its 16 bytes;
//   - a NULL or empty FK value derives NULL and is a valid terminal state; and
//   - a NULL, empty, or malformed owned value, or a malformed populated FK value, derives
//     NULL and blocks completion.
//
// Blocking is reported rather than repaired: repairing would mean changing user data, which
// the compact project never does.
// Parameters:
//   - target: registry target whose kind selects the null semantics.
//   - legacy: observed legacy text, and whether the column was non-NULL.
//
// Return values:
//   - nullCompactUUID: the derived shadow value.
//   - bool: true when this observation blocks completion.
func deriveCompactFromLegacy(target compactTarget, legacy nullString) (nullCompactUUID, bool) {
	if !legacy.valid || isBlankLegacyValue(legacy.value) {
		// A missing owned UUID cannot be repaired without inventing user data, so it is a
		// validation blocker; a missing nullable FK is simply absent.
		return nullCompactUUID{}, !target.nullable()
	}
	// Deliberately the untrimmed value: the codec's accept boundary must stay byte-exact so it
	// agrees with each dialect's trigger, which rejects a whitespace-padded UUID.
	value, err := parseCompactUUID(legacy.value)
	if err != nil {
		// Malformed text stays exactly as written and derives NULL. It never aborts the
		// write and never mutates the authoritative column.
		return nullCompactUUID{}, true
	}
	return nullCompactUUID{value: value, valid: true}, false
}

// isBlankLegacyValue reports whether an observed legacy value carries no identifier at all.
//
// This is NOT a convenience wrapper around a trimmed comparison, and the distinction is what
// makes it correct on every dialect. The legacy UUID columns are CHAR(36), and PostgreSQL's
// CHAR is bpchar: it physically space-pads every shorter value to 36 characters and returns the
// padded form through the driver. An empty nullable FK therefore reads back in Go as 36 spaces,
// not as "". A plain `value == ""` test misses that, so the value falls through to the codec,
// fails to parse, and is classified as a malformed populated FK — which blocks completion
// forever over data the proposal defines as a valid terminal state. MySQL's CHAR strips trailing
// spaces on retrieval and SQLite never pads, so the bug is invisible on both.
//
// Blankness is deliberately the ONLY place trimming happens. The codec itself must stay
// byte-exact, because its accept boundary has to match each dialect's trigger, and those reject
// a whitespace-padded UUID. So " 018f0000-..." is not blank, reaches the codec, and is rejected
// exactly as the trigger rejects it.
// Parameters:
//   - value: observed legacy text, exactly as the driver returned it.
//
// Return values:
//   - bool: true when the value is empty or consists only of whitespace.
func isBlankLegacyValue(value string) bool {
	return strings.TrimSpace(value) == ""
}

// nullString is the observed state of one authoritative legacy text column.
//
// SQL NULL and empty text are deliberately distinct here even though both derive compact
// NULL: the raw-source fingerprint stream must prove the legacy states were observed without
// conflation, and the API must keep rendering the legacy value exactly as before.
type nullString struct {
	// value is the observed text when valid is true.
	value string
	// valid reports whether the column was non-NULL.
	valid bool
}

// Scan implements sql.Scanner for an authoritative legacy text column.
// Parameters:
//   - src: driver value for the legacy column.
//
// Return values:
//   - error: wrapped error when the value has an unexpected driver type.
func (null *nullString) Scan(src any) error {
	*null = nullString{}
	switch typed := src.(type) {
	case nil:
		return nil
	case string:
		null.value = typed
		null.valid = true
		return nil
	case []byte:
		// Copy: the driver may reuse the buffer after the next scan.
		null.value = string(typed)
		null.valid = true
		return nil
	default:
		return errors.Errorf("legacy uuid column has unsupported driver type %T", src)
	}
}

// equal reports whether two derived shadow observations are semantically identical.
// Parameters:
//   - other: shadow value to compare against.
//
// Return values:
//   - bool: true when both are NULL or both carry the same bytes.
func (null nullCompactUUID) equal(other nullCompactUUID) bool {
	if null.valid != other.valid {
		return false
	}
	if !null.valid {
		return true
	}
	return null.value == other.value
}
