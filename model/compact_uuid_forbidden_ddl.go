package model

// This file is the runtime half of the forbidden-DDL guard (AUTO-016, proposal sections 10.3
// and 13/AUTO-A12).
//
// The compatibility contract makes the legacy text representation permanent. While it is
// active, no command, environment value, hidden phase, or automatic branch may drop, rename,
// retype, change the nullability or collation of, or repurpose a legacy UUID column or its
// index. An attempted cleanup must fail BEFORE any DDL reaches the database.
//
// This guard is deliberately belt-and-braces with the static assertion in the test suite. The
// static check proves no production path contains such a statement today; this runtime check
// makes a statement that somehow reached the executor fail before it runs. Neither alone is
// sufficient: a static check cannot see a dynamically built string, and a runtime check cannot
// prove absence.

import (
	"strings"

	"github.com/Laisky/errors/v2"
)

// errForbiddenLegacyDDL is the sentinel for a rejected destructive statement.
var errForbiddenLegacyDDL = errors.New(
	"destructive legacy uuid DDL is prohibited while the compact compatibility contract is active")

// compactGuardedDDL screens one trusted DDL statement before it reaches the database.
//
// It is applied to compact migration DDL, which is built only from the compile-time registry
// and therefore should never trip it. That is the point: if it ever does trip, a generator
// changed in a way that violates the contract, and failing loudly before DDL is exactly the
// required behavior.
// Parameters:
//   - statement: trusted DDL statement built from registry identifiers.
//
// Return values:
//   - error: errForbiddenLegacyDDL-wrapped error when the statement would destroy legacy
//     storage; nil when the statement is additive and safe.
func compactGuardedDDL(statement string) error {
	normalized := strings.Join(strings.Fields(strings.ToLower(statement)), " ")
	for _, target := range compactRegistry() {
		if err := screenLegacyColumnDDL(normalized, target); err != nil {
			return err
		}
	}
	return nil
}

// screenLegacyColumnDDL rejects a statement that would destroy one target's legacy storage.
//
// The screen is intentionally shape-based rather than a parser. It looks for the specific
// destructive verbs applied to a legacy identifier, because those are the operations the
// contract names, and a false positive here fails a migration loudly rather than silently
// destroying a column.
// Parameters:
//   - normalized: whitespace-normalized lowercase statement.
//   - target: registry target whose legacy identifiers are protected.
//
// Return values:
//   - error: errForbiddenLegacyDDL-wrapped error when the statement is destructive.
func screenLegacyColumnDDL(normalized string, target compactTarget) error {
	legacy := strings.ToLower(target.legacyColumn)

	for _, verb := range []string{
		"drop column",
		"drop index",
		"rename column",
		"modify column",
		"alter column",
		"change column",
	} {
		index := strings.Index(normalized, verb)
		if index < 0 {
			continue
		}
		operand := normalized[index+len(verb):]
		if !mentionsIdentifier(operand, legacy) {
			// Operating on the compact shadow alone is this migration's own additive object
			// and is permitted. No extra allowance is needed to express that: because
			// mentionsIdentifier matches whole tokens, `DROP COLUMN uuid_compact` simply does
			// not mention the legacy `uuid` token and never reaches the rejection below.
			//
			// An earlier revision did carry such an allowance — "skip when the operand also
			// mentions the compact column" — and it was a false-negative hole rather than a
			// convenience. It let `RENAME COLUMN uuid TO uuid_compact` and
			// `DROP COLUMN uuid_compact, DROP COLUMN uuid` through, both of which destroy the
			// authoritative column. Do not reintroduce it.
			continue
		}
		return errors.Wrapf(errForbiddenLegacyDDL,
			"statement would %s the authoritative legacy column or index for %s", verb, target.id())
	}
	return nil
}

// mentionsIdentifier reports whether an operand references an identifier as a whole token.
//
// Whole-token matching matters because `uuid` is a prefix of `uuid_compact`: a substring test
// would flag every legitimate compact statement as an attack on the legacy column.
// Parameters:
//   - operand: lowercase statement fragment following a verb.
//   - identifier: lowercase identifier to look for.
//
// Return values:
//   - bool: true when the operand references the identifier as a complete token.
func mentionsIdentifier(operand string, identifier string) bool {
	for _, token := range strings.FieldsFunc(operand, func(char rune) bool {
		switch char {
		case ' ', ',', '(', ')', ';', '`', '"', '\'', '\t', '\n':
			return true
		default:
			return false
		}
	}) {
		if token == identifier {
			return true
		}
	}
	return false
}
