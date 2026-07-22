package state

// OwnerScope identifies the tenant that owns a state record. Every store lookup
// validates the full (UserID, TokenID) pair before returning a record; an unknown
// ID and a foreign-owner ID return the same external not-found error to avoid
// enumeration (SEC03).
type OwnerScope struct {
	UserID  int `json:"user_id"`
	TokenID int `json:"token_id"`
}

// Valid reports whether the owner scope is usable for a lookup. A user ID is
// always required; a token ID of zero is permitted for internal callers that are
// not scoped to a specific token.
func (o OwnerScope) Valid() bool {
	return o.UserID > 0
}

// Matches reports whether other refers to the same tenant. Both the user ID and
// the token ID must match: a record created under one token is not readable
// through another token of the same user, which keeps per-token isolation intact.
func (o OwnerScope) Matches(other OwnerScope) bool {
	return o.UserID == other.UserID && o.TokenID == other.TokenID
}
