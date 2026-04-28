package model

// UserMetadata stores per-user toggles that don't justify their own column.
// Add new fields here freely; the column is JSON-encoded TEXT.
type UserMetadata struct {
	PasswordLocked bool `json:"password_locked,omitempty"`
}
