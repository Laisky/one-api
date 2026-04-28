package model

import (
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/logger"
)

// MCPServerSortFields enumerates whitelisted columns for MCP server sorting.
var MCPServerSortFields = map[string]string{
	"id":         "id",
	"name":       "name",
	"status":     "status",
	"priority":   "priority",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

// ListMCPServers returns MCP servers with pagination and sorting applied.
func ListMCPServers(offset int, limit int, sortBy string, sortOrder string) ([]*MCPServer, error) {
	query := DB.Model(&MCPServer{})
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	orderClause := ValidateOrderClause(sortBy, sortOrder, MCPServerSortFields, "id desc")
	query = query.Order(orderClause)

	var servers []*MCPServer
	if err := query.Find(&servers).Error; err != nil {
		return nil, errors.Wrap(err, "list mcp servers")
	}
	if err := decryptMCPServerSecrets(servers); err != nil {
		return nil, errors.Wrap(err, "decrypt mcp server secrets")
	}
	return servers, nil
}

// CountMCPServers returns the total number of MCP servers.
func CountMCPServers() (int64, error) {
	var count int64
	if err := DB.Model(&MCPServer{}).Count(&count).Error; err != nil {
		return 0, errors.Wrap(err, "count mcp servers")
	}
	return count, nil
}

// GetMCPServerByID fetches a MCP server by identifier.
func GetMCPServerByID(id int) (*MCPServer, error) {
	if id <= 0 {
		return nil, errors.New("mcp server id is invalid")
	}
	server := MCPServer{Id: id}
	if err := DB.First(&server, "id = ?", id).Error; err != nil {
		return nil, errors.Wrap(err, "get mcp server")
	}
	if err := decryptMCPServerSecret(&server); err != nil {
		return nil, errors.Wrap(err, "decrypt mcp server secret")
	}
	return &server, nil
}

// CreateMCPServer persists a new MCP server record.
func CreateMCPServer(server *MCPServer) error {
	if server == nil {
		return errors.New("mcp server is nil")
	}
	if err := server.NormalizeAndValidate(); err != nil {
		return errors.Wrap(err, "normalize and validate mcp server")
	}
	if err := encryptMCPServerSecret(server); err != nil {
		return errors.Wrap(err, "encrypt mcp server secret")
	}
	if err := DB.Create(server).Error; err != nil {
		return errors.Wrap(err, "create mcp server")
	}
	return nil
}

// UpdateMCPServer updates an existing MCP server record.
func UpdateMCPServer(server *MCPServer) error {
	if server == nil {
		return errors.New("mcp server is nil")
	}
	if server.Id <= 0 {
		return errors.New("mcp server id is invalid")
	}
	if err := server.NormalizeAndValidate(); err != nil {
		return errors.Wrap(err, "normalize and validate mcp server")
	}
	if err := encryptMCPServerSecret(server); err != nil {
		return errors.Wrap(err, "encrypt mcp server secret")
	}
	if err := DB.Model(server).Updates(server).Error; err != nil {
		return errors.Wrap(err, "update mcp server")
	}
	// GORM's struct-based Updates silently skips zero-value fields (empty
	// strings, false, nil maps/slices, zero numbers). When the UI lets the
	// user clear a field, sending "" or {} or false would otherwise leave
	// the previous value untouched. The controller layer records which
	// columns were present in the raw request body so we can issue per-
	// column updates that respect zero values for those fields.
	if len(server.ProvidedFields) > 0 {
		columnValues := map[string]any{
			"description":                server.Description,
			"api_key":                    server.APIKey,
			"headers":                    server.Headers,
			"tool_whitelist":             server.ToolWhitelist,
			"tool_blacklist":             server.ToolBlacklist,
			"tool_pricing":               server.ToolPricing,
			"auto_sync_enabled":          server.AutoSyncEnabled,
			"auto_sync_interval_minutes": server.AutoSyncIntervalMinutes,
			"priority":                   server.Priority,
			"status":                     server.Status,
			"base_url":                   server.BaseURL,
			"protocol":                   server.Protocol,
			"auth_type":                  server.AuthType,
			"name":                       server.Name,
			"last_sync_status":           server.LastSyncStatus,
			"last_sync_error":            server.LastSyncError,
			"last_test_status":           server.LastTestStatus,
			"last_test_error":            server.LastTestError,
		}
		forcedUpdates := make(map[string]any, len(server.ProvidedFields))
		cleared := make([]string, 0, len(server.ProvidedFields))
		for column, value := range columnValues {
			if !server.ProvidedFields[column] {
				continue
			}
			forcedUpdates[column] = value
			if isZeroForUpdate(value) {
				cleared = append(cleared, column)
			}
		}
		if len(forcedUpdates) > 0 {
			if err := DB.Model(&MCPServer{}).Where("id = ?", server.Id).Updates(forcedUpdates).Error; err != nil {
				return errors.Wrapf(err, "update provided fields for mcp server id=%d", server.Id)
			}
			if len(cleared) > 0 && logger.Logger != nil {
				// Field NAMES only, never values (api_key, headers may contain secrets).
				logger.Logger.Debug("mcp server update cleared fields",
					zap.Int("mcp_server_id", server.Id),
					zap.Strings("cleared_fields", cleared))
			}
		}
	}
	return nil
}

// isZeroForUpdate reports whether a value is the zero/empty form that GORM's
// struct-based Updates would skip. Used only for DEBUG logging of cleared
// field names.
func isZeroForUpdate(v any) bool {
	switch x := v.(type) {
	case string:
		return x == ""
	case int:
		return x == 0
	case int64:
		return x == 0
	case bool:
		return !x
	case JSONStringMap:
		return len(x) == 0
	case JSONStringSlice:
		return len(x) == 0
	case MCPToolPricingMap:
		return len(x) == 0
	}
	return false
}

// DeleteMCPServer deletes a MCP server record by id.
func DeleteMCPServer(id int) error {
	if id <= 0 {
		return errors.New("mcp server id is invalid")
	}
	if err := DB.Delete(&MCPServer{}, id).Error; err != nil {
		return errors.Wrap(err, "delete mcp server")
	}
	return nil
}

// GetMCPServerByName fetches an MCP server by name.
func GetMCPServerByName(name string) (*MCPServer, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.New("mcp server name is required")
	}
	server := MCPServer{}
	if err := DB.First(&server, "name = ?", trimmed).Error; err != nil {
		return nil, errors.Wrap(err, "get mcp server by name")
	}
	if err := decryptMCPServerSecret(&server); err != nil {
		return nil, errors.Wrap(err, "decrypt mcp server secret")
	}
	return &server, nil
}

// ListEnabledMCPServers returns enabled MCP servers.
func ListEnabledMCPServers() ([]*MCPServer, error) {
	var servers []*MCPServer
	if err := DB.Where("status = ?", MCPServerStatusEnabled).Find(&servers).Error; err != nil {
		return nil, errors.Wrap(err, "list enabled mcp servers")
	}
	if err := decryptMCPServerSecrets(servers); err != nil {
		return nil, errors.Wrap(err, "decrypt enabled mcp server secrets")
	}
	return servers, nil
}

// encryptMCPServerSecret encrypts the API key before persisting.
func encryptMCPServerSecret(server *MCPServer) error {
	if server == nil {
		return errors.New("mcp server is nil")
	}
	if server.APIKey == "" {
		return nil
	}
	encoded, err := common.EncryptSecret(server.APIKey)
	if err != nil {
		return errors.Wrap(err, "encrypt mcp server api key")
	}
	server.APIKey = encoded
	return nil
}

// decryptMCPServerSecrets decrypts API keys for a list of MCP servers.
func decryptMCPServerSecrets(servers []*MCPServer) error {
	for _, server := range servers {
		if err := decryptMCPServerSecret(server); err != nil {
			return errors.Wrap(err, "decrypt mcp server secret in batch")
		}
	}
	return nil
}

// decryptMCPServerSecret decrypts API key for a single MCP server.
func decryptMCPServerSecret(server *MCPServer) error {
	if server == nil {
		return errors.New("mcp server is nil")
	}
	if server.APIKey == "" {
		return nil
	}
	decoded, err := common.DecryptSecret(server.APIKey)
	if err != nil {
		return errors.Wrap(err, "decrypt mcp server api key")
	}
	server.APIKey = decoded
	return nil
}
