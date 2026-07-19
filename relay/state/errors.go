package state

import "github.com/Laisky/errors/v2"

// Sentinel errors returned by ResponseStateStore implementations. They are mapped
// to the stable machine-readable API error codes in Section 6 of the proposal by
// the controller layer; callers compare with errors.Is.
var (
	// ErrNotFound is returned for an unknown ID, a foreign-owner ID, or a
	// tombstoned ID. The external shape is identical for all three so ownership
	// cannot be probed (SEC03, E03).
	ErrNotFound = errors.New("state: record not found")

	// ErrVersionConflict is returned when a compare-and-set append or update is
	// attempted against a stale version.
	ErrVersionConflict = errors.New("state: version conflict")

	// ErrLeaseHeld is returned when another request currently owns a conversation
	// mutation lease (maps to conversation_conflict / HTTP 409).
	ErrLeaseHeld = errors.New("state: conversation lease held")

	// ErrLeaseInvalid is returned when a lease token does not match the current
	// lease holder (already expired, released, or never acquired).
	ErrLeaseInvalid = errors.New("state: conversation lease invalid")

	// ErrLimitExceeded is returned when a record exceeds a configured bound before
	// it is written (maps to state_limit_exceeded / HTTP 413).
	ErrLimitExceeded = errors.New("state: limit exceeded")

	// ErrUnsupportedSchema is returned when a stored record carries a schema
	// version this build cannot decode; the caller must surface a typed migration
	// error rather than silently dropping fields (S07).
	ErrUnsupportedSchema = errors.New("state: unsupported schema version")

	// ErrStoreUnavailable is returned when the shared store cannot be read or
	// committed safely (maps to state_store_unavailable / HTTP 503).
	ErrStoreUnavailable = errors.New("state: store unavailable")

	// ErrInvalidOwner is returned when an OwnerScope is not usable for a lookup.
	ErrInvalidOwner = errors.New("state: invalid owner scope")
)
