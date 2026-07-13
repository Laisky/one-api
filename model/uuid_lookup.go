package model

import (
	"strings"

	"github.com/Laisky/errors/v2"
)

// normalizeUUIDKeyword canonicalizes a search keyword for exact UUID matching.
// Server-generated UUIDs are stored as lowercase hyphenated strings, so trimming
// surrounding whitespace and lowercasing lets a pasted UUID match regardless of
// case or padding, uniformly across MySQL, PostgreSQL, and SQLite.
//
// Parameters:
//   - keyword: raw search keyword supplied by the request.
//
// Return values:
//   - string: trimmed, lowercased keyword suitable for a `uuid = ?` comparison.
func normalizeUUIDKeyword(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}

// GetUserByUUID retrieves a user by its external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - *User: matched user with sensitive fields omitted.
//   - error: wrapped database error when lookup fails.
func GetUserByUUID(uuid string) (*User, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.New("uuid is empty")
	}
	user := &User{}
	if err := DB.Omit("password", "access_token").First(user, "uuid = ?", uuid).Error; err != nil {
		return nil, errors.Wrapf(err, "get user by uuid %s", uuid)
	}
	return user, nil
}

// GetUserIdByUUID retrieves a user's internal id by external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - int: internal primary key.
//   - error: wrapped database error when lookup fails.
func GetUserIdByUUID(uuid string) (int, error) {
	user, err := GetUserByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return user.Id, nil
}

// GetUserUUIDByID retrieves a user's external UUID by internal id.
// Parameters:
//   - id: internal user primary key.
//
// Return values:
//   - string: external UUID, or an empty string when the row has not been backfilled yet.
//   - error: wrapped database error when lookup fails.
func GetUserUUIDByID(id int) (string, error) {
	if id <= 0 {
		return "", errors.New("user id is invalid")
	}
	user := &User{}
	if err := DB.Select("uuid").First(user, "id = ?", id).Error; err != nil {
		return "", errors.Wrapf(err, "get user uuid by id %d", id)
	}
	return user.UUID, nil
}

// GetChannelByUUID retrieves a channel by its external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - *Channel: matched channel.
//   - error: wrapped database error when lookup fails.
func GetChannelByUUID(uuid string) (*Channel, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.New("uuid is empty")
	}
	channel := &Channel{}
	if err := DB.First(channel, "uuid = ?", uuid).Error; err != nil {
		return nil, errors.Wrapf(err, "get channel by uuid %s", uuid)
	}
	return channel, nil
}

// GetChannelIdByUUID retrieves a channel's internal id by external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - int: internal primary key.
//   - error: wrapped database error when lookup fails.
func GetChannelIdByUUID(uuid string) (int, error) {
	channel, err := GetChannelByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return channel.Id, nil
}

// GetChannelUUIDByID retrieves a channel's external UUID by internal id.
// Parameters:
//   - id: internal channel primary key.
//
// Return values:
//   - string: external UUID, or an empty string when the row has not been backfilled yet.
//   - error: wrapped database error when lookup fails.
func GetChannelUUIDByID(id int) (string, error) {
	if id <= 0 {
		return "", errors.New("channel id is invalid")
	}
	channel := &Channel{}
	if err := DB.Select("uuid").First(channel, "id = ?", id).Error; err != nil {
		return "", errors.Wrapf(err, "get channel uuid by id %d", id)
	}
	return channel.UUID, nil
}

// GetTokenByUUID retrieves a token by its external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - *Token: matched token.
//   - error: wrapped database error when lookup fails.
func GetTokenByUUID(uuid string) (*Token, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.New("uuid is empty")
	}
	token := &Token{}
	if err := DB.First(token, "uuid = ?", uuid).Error; err != nil {
		return nil, errors.Wrapf(err, "get token by uuid %s", uuid)
	}
	return token, nil
}

// GetTokenIdByUUID retrieves a token's internal id by external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - int: internal primary key.
//   - error: wrapped database error when lookup fails.
func GetTokenIdByUUID(uuid string) (int, error) {
	token, err := GetTokenByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return token.Id, nil
}

// GetTokenUUIDByID retrieves a token's external UUID by internal id.
// Parameters:
//   - id: internal token primary key.
//
// Return values:
//   - string: external UUID, or an empty string when the row has not been backfilled yet.
//   - error: wrapped database error when lookup fails.
func GetTokenUUIDByID(id int) (string, error) {
	if id <= 0 {
		return "", errors.New("token id is invalid")
	}
	token := &Token{}
	if err := DB.Select("uuid").First(token, "id = ?", id).Error; err != nil {
		return "", errors.Wrapf(err, "get token uuid by id %d", id)
	}
	return token.UUID, nil
}

// GetRedemptionByUUID retrieves a redemption code by its external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - *Redemption: matched redemption.
//   - error: wrapped database error when lookup fails.
func GetRedemptionByUUID(uuid string) (*Redemption, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.New("uuid is empty")
	}
	redemption := &Redemption{}
	if err := DB.First(redemption, "uuid = ?", uuid).Error; err != nil {
		return nil, errors.Wrapf(err, "get redemption by uuid %s", uuid)
	}
	return redemption, nil
}

// GetRedemptionIdByUUID retrieves a redemption's internal id by external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - int: internal primary key.
//   - error: wrapped database error when lookup fails.
func GetRedemptionIdByUUID(uuid string) (int, error) {
	redemption, err := GetRedemptionByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return redemption.Id, nil
}

// GetLogByUUID retrieves a log row by its external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - *Log: matched log.
//   - error: wrapped database error when lookup fails.
func GetLogByUUID(uuid string) (*Log, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.New("uuid is empty")
	}
	log := &Log{}
	if err := LOG_DB.First(log, "uuid = ?", uuid).Error; err != nil {
		return nil, errors.Wrapf(err, "get log by uuid %s", uuid)
	}
	return log, nil
}

// GetLogIdByUUID retrieves a log's internal id by external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - int: internal primary key.
//   - error: wrapped database error when lookup fails.
func GetLogIdByUUID(uuid string) (int, error) {
	log, err := GetLogByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return log.Id, nil
}

// GetMCPServerByUUID retrieves an MCP server by its external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - *MCPServer: matched server.
//   - error: wrapped database error when lookup fails.
func GetMCPServerByUUID(uuid string) (*MCPServer, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.New("uuid is empty")
	}
	server := &MCPServer{}
	if err := DB.First(server, "uuid = ?", uuid).Error; err != nil {
		return nil, errors.Wrapf(err, "get mcp server by uuid %s", uuid)
	}
	return server, nil
}

// GetMCPServerIdByUUID retrieves an MCP server's internal id by external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - int: internal primary key.
//   - error: wrapped database error when lookup fails.
func GetMCPServerIdByUUID(uuid string) (int, error) {
	server, err := GetMCPServerByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return server.Id, nil
}

// GetPasskeyCredentialByUUID retrieves a passkey credential by external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - *PasskeyCredential: matched credential.
//   - error: wrapped database error when lookup fails.
func GetPasskeyCredentialByUUID(uuid string) (*PasskeyCredential, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil, errors.New("uuid is empty")
	}
	credential := &PasskeyCredential{}
	if err := DB.First(credential, "uuid = ?", uuid).Error; err != nil {
		return nil, errors.Wrapf(err, "get passkey credential by uuid %s", uuid)
	}
	return credential, nil
}

// GetPasskeyCredentialIdByUUID retrieves a passkey credential's internal id by external UUID.
// Parameters:
//   - uuid: canonical hyphenated UUID string.
//
// Return values:
//   - int: internal primary key.
//   - error: wrapped database error when lookup fails.
func GetPasskeyCredentialIdByUUID(uuid string) (int, error) {
	credential, err := GetPasskeyCredentialByUUID(uuid)
	if err != nil {
		return 0, err
	}
	return credential.Id, nil
}
