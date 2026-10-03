package model

// Shared normalized log-list filter (proposal
// docs/proposals/20260905_observability-data-tiering.md, W2.4).
//
// The legacy offset list and the additive cursor list must select exactly the
// same rows for the same request, or the cursor capability would quietly show a
// different history. That is only guaranteed if both build their predicate from
// one place, so this file owns the normalization, the canonical digest bound
// into a cursor, and the WHERE fragment itself.
//
// Nothing here changes legacy behavior: the predicate is a transcription of the
// conditions model.GetAllLogs / GetUserLogs already apply, including the
// INCLUSIVE `created_at <= end` bound and the unconditional provisional
// exclusion, and LogListFilterEqualsLegacy pins that equivalence.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// LogListScopeKind identifies whose rows a list request may return.
type LogListScopeKind string

const (
	// LogListScopeSelf restricts a listing to a single subject user.
	LogListScopeSelf LogListScopeKind = "self"
	// LogListScopeAll is the site-wide administrative listing.
	LogListScopeAll LogListScopeKind = "all"
)

// LogListScope is the effective authorization scope of one list request.
//
// It is always derived from the authenticated principal on the current request.
// A cursor may carry a DIGEST of it for consistency checking, but never the
// scope itself: a cursor must not be able to widen access.
type LogListScope struct {
	// Kind is self or all.
	Kind LogListScopeKind
	// SubjectUserID is the user whose rows are returned; 0 when Kind is all.
	SubjectUserID int
	// PrincipalUserID is the authenticated caller.
	PrincipalUserID int
	// Role is the caller's effective role on this request.
	Role int
}

// LogListFilter is the normalized set of user-supplied list filters.
//
// Field semantics are identical to the legacy list helpers, including the
// zero-value "unset" convention for LogType, timestamps and ChannelID.
type LogListFilter struct {
	// LogType selects a single log type; LogTypeUnknown means every type.
	LogType int
	// StartTimestamp is an INCLUSIVE lower bound on created_at, Unix seconds.
	StartTimestamp int64
	// EndTimestamp is an INCLUSIVE upper bound on created_at, Unix seconds.
	EndTimestamp int64
	// ModelName is an exact match when non-empty.
	ModelName string
	// Username is an exact match when non-empty; administrative scope only.
	Username string
	// TokenName is an exact match when non-empty.
	TokenName string
	// ChannelID is an exact match when non-zero; administrative scope only.
	ChannelID int
}

// maxLogFilterValueBytes bounds each free-text filter so a caller cannot force
// an unbounded digest input or an oversized statement argument.
const maxLogFilterValueBytes = 255

// Normalize returns the filter in the canonical form both list paths use.
//
// Normalization is idempotent, so a value that survives one pass digests
// identically on the next. Under self scope the administrative-only fields are
// cleared rather than rejected, matching the legacy handlers, which simply never
// read them on that route.
//
// Parameters:
//   - scope: the effective authorization scope for this request.
//
// Return values:
//   - LogListFilter: the normalized filter.
func (f LogListFilter) Normalize(scope LogListScope) LogListFilter {
	out := f

	out.ModelName = normalizeLogFilterValue(out.ModelName)
	out.Username = normalizeLogFilterValue(out.Username)
	out.TokenName = normalizeLogFilterValue(out.TokenName)

	if out.LogType < 0 {
		out.LogType = LogTypeUnknown
	}
	if out.StartTimestamp < 0 {
		out.StartTimestamp = 0
	}
	if out.EndTimestamp < 0 {
		out.EndTimestamp = 0
	}
	if out.ChannelID < 0 {
		out.ChannelID = 0
	}

	if scope.Kind == LogListScopeSelf {
		// These are administrative-only filters; the self route never applies
		// them, so they must not affect its digest either.
		out.Username = ""
		out.ChannelID = 0
	}

	return out
}

// normalizeLogFilterValue trims and bounds one free-text filter value.
//
// Parameters:
//   - value: the raw value.
//
// Return values:
//   - string: the trimmed value, truncated to maxLogFilterValueBytes.
func normalizeLogFilterValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > maxLogFilterValueBytes {
		return trimmed[:maxLogFilterValueBytes]
	}
	return trimmed
}

// logCursorSortCreatedAtDesc is the only cursor sort this version supports.
const logCursorSortCreatedAtDesc = "created_at:desc"

// logFilterDigestDomain separates this digest from every other use of SHA-256
// in the project.
const logFilterDigestDomain = "one-api/log-cursor/filter/v1\x00"

// Digest returns the canonical digest of a normalized filter, its scope and the
// fixed contract choices a cursor is bound to.
//
// The encoding is length-prefixed so that no two different field sequences can
// produce the same byte string: without the prefixes, ModelName "ab" with
// Username "c" would digest identically to ModelName "a" with Username "bc".
//
// Page size is deliberately NOT included. Keyset pagination stays correct when
// the page size changes, so binding it would only manufacture spurious
// restart-required responses when a user adjusts the page-size selector.
//
// Parameters:
//   - scope: the effective authorization scope.
//   - endpoint: the route identifier, so a cursor cannot cross endpoints.
//
// Return values:
//   - string: lowercase hex SHA-256 of the canonical encoding.
func (f LogListFilter) Digest(scope LogListScope, endpoint string) string {
	sum := f.DigestBytes(scope, endpoint)
	return hex.EncodeToString(sum[:])
}

// DigestBytes returns the raw digest a cursor seal binds.
//
// The seal needs the bytes and the cache key needs the hex, so the bytes are
// the primitive: deriving them back out of the hex string would add a decode
// that can fail, for a value that never left the process.
//
// Parameters:
//   - scope: the effective authorization scope.
//   - endpoint: the route identifier, so a cursor cannot cross endpoints.
//
// Return values:
//   - [32]byte: SHA-256 of the canonical encoding.
func (f LogListFilter) DigestBytes(scope LogListScope, endpoint string) [32]byte {
	var buf []byte

	buf = appendDigestString(buf, 1, endpoint)
	buf = appendDigestString(buf, 2, logCursorSortCreatedAtDesc)
	buf = appendDigestString(buf, 3, string(scope.Kind))
	buf = appendDigestInt(buf, 4, int64(scope.SubjectUserID))
	buf = appendDigestInt(buf, 5, int64(scope.PrincipalUserID))
	buf = appendDigestInt(buf, 6, int64(scope.Role))
	buf = appendDigestInt(buf, 7, int64(f.LogType))
	buf = appendDigestInt(buf, 8, f.StartTimestamp)
	buf = appendDigestInt(buf, 9, f.EndTimestamp)
	// The end bound is inclusive, matching the legacy wire contract. Recording
	// the flavour lets a half-open mode be added later without silently
	// reinterpreting cursors already issued under this one.
	buf = appendDigestInt(buf, 10, 0)
	buf = appendDigestString(buf, 11, f.ModelName)
	buf = appendDigestString(buf, 12, f.Username)
	buf = appendDigestString(buf, 13, f.TokenName)
	buf = appendDigestInt(buf, 14, int64(f.ChannelID))
	// Provisional exclusion is not optional; it is recorded so a future opt-in
	// would change the digest rather than silently reusing cursors.
	buf = appendDigestInt(buf, 15, 1)

	return sha256.Sum256(append([]byte(logFilterDigestDomain), buf...))
}

// appendDigestString appends a tagged, length-prefixed string field.
//
// Parameters:
//   - buf: the buffer being built.
//   - tag: the field tag.
//   - value: the field value.
//
// Return values:
//   - []byte: buf with the field appended.
func appendDigestString(buf []byte, tag byte, value string) []byte {
	buf = append(buf, tag)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(value)))
	return append(buf, value...)
}

// appendDigestInt appends a tagged fixed-width integer field.
//
// Parameters:
//   - buf: the buffer being built.
//   - tag: the field tag.
//   - value: the field value.
//
// Return values:
//   - []byte: buf with the field appended.
func appendDigestInt(buf []byte, tag byte, value int64) []byte {
	buf = append(buf, tag)
	return binary.BigEndian.AppendUint64(buf, uint64(value))
}

// BuildLogListPredicate renders the shared WHERE fragment and its arguments.
//
// The fragment is a transcription of the conditions the legacy list helpers
// apply, in the same order, so both paths select the same rows. It always
// excludes provisional rows and always requires a non-null created_at: that
// column is nullable, and PostgreSQL orders NULLs first under DESC while SQLite
// and MySQL order them last, so such a row has no engine-independent position in
// a keyset traversal.
//
// Parameters:
//   - scope: the effective authorization scope.
//   - filter: the NORMALIZED filter.
//
// Return values:
//   - string: the WHERE fragment, without the leading WHERE keyword.
//   - []any: the ordered arguments for its placeholders.
func BuildLogListPredicate(scope LogListScope, filter LogListFilter) (string, []any) {
	conditions := make([]string, 0, 10)
	args := make([]any, 0, 10)

	conditions = append(conditions, "type <> ?")
	args = append(args, LogTypeProvisional)

	conditions = append(conditions, "created_at IS NOT NULL")

	if scope.Kind == LogListScopeSelf {
		conditions = append(conditions, "user_id = ?")
		args = append(args, scope.SubjectUserID)
	}
	if filter.LogType != LogTypeUnknown {
		conditions = append(conditions, "type = ?")
		args = append(args, filter.LogType)
	}
	if filter.ModelName != "" {
		conditions = append(conditions, "model_name = ?")
		args = append(args, filter.ModelName)
	}
	if filter.Username != "" {
		conditions = append(conditions, "username = ?")
		args = append(args, filter.Username)
	}
	if filter.TokenName != "" {
		conditions = append(conditions, "token_name = ?")
		args = append(args, filter.TokenName)
	}
	if filter.StartTimestamp != 0 {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, filter.StartTimestamp)
	}
	if filter.EndTimestamp != 0 {
		// INCLUSIVE, matching the legacy list wire contract.
		conditions = append(conditions, "created_at <= ?")
		args = append(args, filter.EndTimestamp)
	}
	if filter.ChannelID != 0 {
		conditions = append(conditions, "channel_id = ?")
		args = append(args, filter.ChannelID)
	}

	return strings.Join(conditions, " AND "), args
}
