// Package groupname defines the single naming rule for user groups.
//
// A user group is the identifier shared by the GroupRatio billing catalog,
// users.group, channels.group (comma-separated) and abilities.group. Names are
// case-preserving and always matched exactly, but two catalog names may not
// differ only by case: abilities.group is part of the abilities primary key and
// follows the database's default collation, which is case-insensitive on MySQL,
// so "VIP" and "vip" cannot coexist on one channel there.
//
// The rule is enforced on write paths only. Stored legacy names that predate it
// keep working on every read path so existing deployments are not broken.
// The Modern frontend mirrors this rule in web/modern/src/lib/group-name.ts, and
// both sides are checked against web/modern/src/lib/testdata/group-names.json.
package groupname

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

// MaxLength is the longest allowed group name in characters. It matches the
// varchar(32) users.group and abilities.group columns.
const MaxLength = 32

// Default is the built-in group every user starts in. It is always assignable,
// even when the GroupRatio catalog omits it (its ratio then falls back to 1).
const Default = "default"

// namePattern lists the allowed characters: ASCII letters, digits, '.', '_' and '-'.
// Whitespace and ',' (the channels.group list separator) are deliberately excluded.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Normalize returns raw with leading and trailing whitespace removed. Callers
// normalize user input before calling Validate; case is never changed.
func Normalize(raw string) string {
	return strings.TrimSpace(raw)
}

// Validate reports whether name satisfies the group naming rule. The name must
// already be normalized: surrounding whitespace is rejected rather than trimmed
// so a stored identifier never silently changes. Returns nil when valid, or an
// error whose message is safe to show to an administrator.
func Validate(name string) error {
	switch {
	case name == "":
		return errors.New("group name cannot be empty")
	case name != strings.TrimSpace(name):
		return errors.Errorf("group name %q must not start or end with whitespace", name)
	case utf8.RuneCountInString(name) > MaxLength:
		return errors.Errorf("group name %q cannot exceed %d characters", name, MaxLength)
	case !namePattern.MatchString(name):
		return errors.Errorf("group name %q may only contain letters, digits, '.', '_' and '-'", name)
	default:
		return nil
	}
}

// IsValid reports whether name satisfies Validate.
func IsValid(name string) bool {
	return Validate(name) == nil
}

// FoldKey returns the case-insensitive identity of name. Two names with the same
// FoldKey would collide in a MySQL abilities primary key.
func FoldKey(name string) string {
	return strings.ToLower(name)
}

// ParseList splits a comma-separated group list as stored in channels.group. It
// trims every entry, drops empty ones and keeps only the first spelling of names
// that differ only by case, which preserves the legacy ability primary-key
// behavior. It returns nil when the list holds no names.
func ParseList(csv string) []string {
	parts := strings.Split(csv, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		name := Normalize(part)
		if name == "" {
			continue
		}
		key := FoldKey(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// JoinList joins group names into the comma-separated channels.group format.
func JoinList(names []string) string {
	return strings.Join(names, ",")
}
