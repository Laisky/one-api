package idresolve

import (
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/errkind"
)

var (
	// ErrInvalidRef identifies a malformed external identifier. It is
	// client-caused by definition: the reference came from the request.
	// The mark is transparent, so errors.Is(err, ErrInvalidRef) and the
	// message text are unchanged.
	ErrInvalidRef = errkind.InvalidRequestErr(errors.New("invalid resource reference"))
	// ErrNotFound identifies a well-formed identifier that does not match a row.
	// Also client-caused: the row simply does not exist.
	ErrNotFound = errkind.NotFoundErr(errors.New("resource reference not found"))
)

// Resolve returns the internal integer id for an external UUID reference.
// Parameters:
//   - lookup: function that returns the internal id for a UUID.
//   - ref: client-supplied UUID identifier; digit-only legacy integer ids are rejected.
//
// Return values:
//   - int: resolved internal primary key.
//   - error: ErrInvalidRef for malformed or legacy integer values, ErrNotFound for unknown UUIDs, or a wrapped lookup error.
func Resolve(lookup func(uuid string) (int, error), ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return 0, ErrInvalidRef
	}

	if !strings.Contains(ref, "-") {
		return 0, ErrInvalidRef
	}

	if lookup == nil {
		return 0, errors.Wrap(ErrInvalidRef, "uuid lookup is nil")
	}
	id, err := lookup(ref)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrNotFound
		}
		return 0, errors.Wrap(err, "resolve uuid reference")
	}
	if id <= 0 {
		return 0, ErrNotFound
	}
	return id, nil
}
