package controller

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/idresolve"
	"github.com/Laisky/one-api/model"
)

// resolveUserRef resolves a user UUID string to an internal id.
// Parameters:
//   - ref: client supplied user reference.
//
// Return values:
//   - int: internal user primary key.
//   - error: invalid-reference or not-found error.
func resolveUserRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetUserIdByUUID, ref)
}

// resolveOptionalUserRef resolves an optional user UUID string.
// Parameters:
//   - ref: client supplied user reference; empty means no filter.
//
// Return values:
//   - int: internal user primary key, or 0 when ref is empty.
//   - error: invalid-reference or not-found error.
func resolveOptionalUserRef(ref string) (int, error) {
	if strings.TrimSpace(ref) == "" {
		return 0, nil
	}
	return resolveUserRef(ref)
}

// resolveChannelRef resolves a channel UUID string to an internal id.
// Parameters:
//   - ref: client supplied channel reference.
//
// Return values:
//   - int: internal channel primary key.
//   - error: invalid-reference or not-found error.
func resolveChannelRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetChannelIdByUUID, ref)
}

// resolveOptionalChannelRef resolves an optional channel UUID string.
// Parameters:
//   - ref: client supplied channel reference; empty means no filter.
//
// Return values:
//   - int: internal channel primary key, or 0 when ref is empty.
//   - error: invalid-reference or not-found error.
func resolveOptionalChannelRef(ref string) (int, error) {
	if strings.TrimSpace(ref) == "" {
		return 0, nil
	}
	return resolveChannelRef(ref)
}

// resolveTokenRef resolves a token UUID string to an internal id.
// Parameters:
//   - ref: client supplied token reference.
//
// Return values:
//   - int: internal token primary key.
//   - error: invalid-reference or not-found error.
func resolveTokenRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetTokenIdByUUID, ref)
}

// resolveRedemptionRef resolves a redemption UUID string to an internal id.
// Parameters:
//   - ref: client supplied redemption reference.
//
// Return values:
//   - int: internal redemption primary key.
//   - error: invalid-reference or not-found error.
func resolveRedemptionRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetRedemptionIdByUUID, ref)
}

// resolveMCPServerRef resolves an MCP server UUID string to an internal id.
// Parameters:
//   - ref: client supplied MCP server reference.
//
// Return values:
//   - int: internal MCP server primary key.
//   - error: invalid-reference or not-found error.
func resolveMCPServerRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetMCPServerIdByUUID, ref)
}

// resolveOptionalMCPServerRef resolves an optional MCP server UUID string.
// Parameters:
//   - ref: client supplied MCP server reference; empty means no filter.
//
// Return values:
//   - int: internal MCP server primary key, or 0 when ref is empty.
//   - error: invalid-reference or not-found error.
func resolveOptionalMCPServerRef(ref string) (int, error) {
	if strings.TrimSpace(ref) == "" {
		return 0, nil
	}
	return resolveMCPServerRef(ref)
}

// resolveLogRef resolves a log UUID string to an internal id.
// Parameters:
//   - ref: client supplied log reference.
//
// Return values:
//   - int: internal log primary key.
//   - error: invalid-reference or not-found error.
func resolveLogRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetLogIdByUUID, ref)
}

// resolvePasskeyCredentialRef resolves a passkey credential UUID string to an internal id.
// Parameters:
//   - ref: client supplied passkey credential reference.
//
// Return values:
//   - int: internal passkey credential primary key.
//   - error: invalid-reference or not-found error.
func resolvePasskeyCredentialRef(ref string) (int, error) {
	return idresolve.Resolve(model.GetPasskeyCredentialIdByUUID, ref)
}

// preferUUIDRef returns a UUID ref for strict-in request bodies.
// Parameters:
//   - uuid: client supplied UUID field.
//   - id: ignored legacy integer id field retained only for decoding old payloads.
//
// Return values:
//   - string: UUID reference string.
//   - error: invalid-reference error when UUID is empty.
func preferUUIDRef(uuid string, id int) (string, error) {
	if strings.TrimSpace(uuid) != "" {
		return uuid, nil
	}
	return "", errors.New("resource uuid is required")
}
