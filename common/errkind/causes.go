package errkind

import "reflect"

// maxErrorCauseNodes bounds malformed cycles and oversized joined-error trees.
const maxErrorCauseNodes = 64

// HasOnlyCause reports whether every leaf of err is the exact target sentinel.
// Transparent wrappers and joins of the same cause are accepted. Mixed, opaque,
// empty, or excessively deep trees remain errors rather than proving completion.
func HasOnlyCause(err, target error) bool {
	if err == nil || target == nil || !reflect.TypeOf(target).Comparable() {
		return false
	}

	pending := []error{err}
	visited := 0
	for len(pending) > 0 {
		if visited == maxErrorCauseNodes {
			return false
		}
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		visited++
		if current == nil {
			return false
		}
		if reflect.TypeOf(current).Comparable() && current == target {
			continue
		}

		switch wrapped := current.(type) {
		case interface{ Unwrap() []error }:
			causes := wrapped.Unwrap()
			if len(causes) == 0 || len(causes) > maxErrorCauseNodes-visited-len(pending) {
				return false
			}
			pending = append(pending, causes...)
		case interface{ Unwrap() error }:
			pending = append(pending, wrapped.Unwrap())
		default:
			return false
		}
	}
	return true
}
