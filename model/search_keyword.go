package model

import (
	"strings"

	"gorm.io/gorm"
)

// uuidCompactLength is the length of a UUID pasted without hyphens (32 hex digits).
const uuidCompactLength = 32

// uuidCanonicalLength is the length of the canonical hyphenated UUID form.
const uuidCanonicalLength = 36

// uuidHyphenPositions lists the indexes that must hold '-' in a canonical UUID.
var uuidHyphenPositions = [4]int{8, 13, 18, 23}

// isHexDigit reports whether r is a lowercase-normalized hexadecimal digit.
//
// Parameters:
//   - r: byte to classify.
//
// Return values:
//   - bool: true when r is 0-9 or a-f.
func isHexDigit(r byte) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}

// canonicalUUIDKeyword reports whether a search keyword is a UUID and returns it in the
// canonical lowercase hyphenated form used by every uuid column in this schema.
//
// Both the hyphenated 36-character form and the 32-hex compact form (a UUID pasted
// without hyphens) are accepted, case-insensitively and with surrounding whitespace
// trimmed. Anything else - including the empty string - is rejected, which is what keeps
// a non-uuid keyword from ever reaching a `uuid = ?` comparison. That matters on
// PostgreSQL, where uuid columns are char(36)/bpchar and trailing-space-insensitive
// comparison would otherwise make an empty keyword match every un-backfilled row.
//
// Parameters:
//   - keyword: raw search keyword supplied by the request.
//
// Return values:
//   - string: canonical lowercase hyphenated UUID, empty when the keyword is not a UUID.
//   - bool: true when the keyword is a UUID and the first return value may be used.
func canonicalUUIDKeyword(keyword string) (string, bool) {
	normalized := normalizeUUIDKeyword(keyword)
	switch len(normalized) {
	case uuidCanonicalLength:
		for i := 0; i < len(normalized); i++ {
			isHyphenSlot := false
			for _, pos := range uuidHyphenPositions {
				if i == pos {
					isHyphenSlot = true
					break
				}
			}
			if isHyphenSlot {
				if normalized[i] != '-' {
					return "", false
				}
				continue
			}
			if !isHexDigit(normalized[i]) {
				return "", false
			}
		}
		return normalized, true
	case uuidCompactLength:
		for i := 0; i < len(normalized); i++ {
			if !isHexDigit(normalized[i]) {
				return "", false
			}
		}
		var sb strings.Builder
		sb.Grow(uuidCanonicalLength)
		sb.WriteString(normalized[0:8])
		sb.WriteByte('-')
		sb.WriteString(normalized[8:12])
		sb.WriteByte('-')
		sb.WriteString(normalized[12:16])
		sb.WriteByte('-')
		sb.WriteString(normalized[16:20])
		sb.WriteByte('-')
		sb.WriteString(normalized[20:32])
		return sb.String(), true
	default:
		return "", false
	}
}

// applyUUIDKeyword narrows a query to rows whose UUID columns equal a pasted UUID.
//
// It is the single place that decides "does this keyword look like a UUID?", so every
// list page shares one matching rule: a UUID keyword matches uuid columns by equality
// (index friendly, no LIKE), and any other keyword is left entirely to the caller's
// existing name/LIKE behaviour. Columns are caller-supplied literals, never user input.
//
// Parameters:
//   - db: query to narrow; never mutated, GORM returns a new statement.
//   - keyword: raw search keyword supplied by the request.
//   - columns: uuid column names to compare, OR'ed together inside one parenthesized
//     group so any pre-existing scope (user_id, provisional exclusion) still ANDs.
//
// Return values:
//   - *gorm.DB: narrowed query when the keyword is a UUID, otherwise db unchanged.
//   - bool: true when the keyword was consumed as a UUID and the caller must not apply
//     its fallback keyword clause.
func applyUUIDKeyword(db *gorm.DB, keyword string, columns ...string) (*gorm.DB, bool) {
	uuid, ok := canonicalUUIDKeyword(keyword)
	if !ok || len(columns) == 0 {
		return db, false
	}
	clauses := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		clauses = append(clauses, column+" = ?")
		args = append(args, uuid)
	}
	return db.Where("("+strings.Join(clauses, " or ")+")", args...), true
}
