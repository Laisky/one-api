package model

import (
	"strings"

	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/random"
)

// ensureUUID assigns a canonical hyphenated UUIDv7 to uuid when it is empty.
// Parameters:
//   - uuid: pointer to the model uuid field that should be generated on create.
//
// Return values:
//   - error: always nil; the signature matches GORM hook requirements.
func ensureUUID(uuid *string) error {
	if uuid == nil {
		return nil
	}
	if strings.TrimSpace(*uuid) == "" {
		*uuid = random.GetUUIDWithHyphens()
	}
	return nil
}

// BeforeCreate assigns a server-generated UUID to a user before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (user *User) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&user.UUID)
}

// BeforeCreate assigns a server-generated UUID to a token before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (t *Token) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&t.UUID)
}

// BeforeCreate assigns a server-generated UUID to a channel before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (channel *Channel) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&channel.UUID)
}

// BeforeCreate assigns a server-generated UUID to a redemption before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (redemption *Redemption) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&redemption.UUID)
}

// BeforeCreate assigns a server-generated UUID to a log before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (log *Log) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&log.UUID)
}

// BeforeCreate assigns a server-generated UUID to a token transaction before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (txn *TokenTransaction) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&txn.UUID)
}

// BeforeCreate assigns a server-generated UUID to a request-cost row before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (cost *UserRequestCost) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&cost.UUID)
}

// BeforeCreate assigns a server-generated UUID to a trace before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (trace *Trace) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&trace.UUID)
}

// BeforeCreate assigns a server-generated UUID to an async task binding before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (binding *AsyncTaskBinding) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&binding.UUID)
}

// BeforeCreate assigns a server-generated UUID to an MCP server before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (server *MCPServer) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&server.UUID)
}

// BeforeCreate assigns a server-generated UUID to an MCP tool before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (tool *MCPTool) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&tool.UUID)
}

// BeforeCreate assigns a server-generated UUID to a passkey credential before insertion.
// Parameters:
//   - tx: GORM transaction supplied by the create callback.
//
// Return values:
//   - error: non-nil only if UUID generation fails.
func (credential *PasskeyCredential) BeforeCreate(tx *gorm.DB) error {
	return ensureUUID(&credential.UUID)
}
