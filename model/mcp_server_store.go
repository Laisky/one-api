package model

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/common"
)

// MCPServerSortFields enumerates sortable columns for MCP server lists.
var MCPServerSortFields = map[string]string{
	"id":         "id",
	"name":       "name",
	"status":     "status",
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

	if sortBy == "" {
		sortBy = "id"
	}
	column, ok := MCPServerSortFields[strings.ToLower(sortBy)]
	if !ok {
		column = "id"
	}
	order := "desc"
	if strings.ToLower(sortOrder) == "asc" {
		order = "asc"
	}
	query = query.Order(column + " " + order)

	var servers []*MCPServer
	if err := query.Find(&servers).Error; err != nil {
		return nil, errors.Wrap(err, "list mcp servers")
	}
	if err := decryptMCPServerSecrets(servers); err != nil {
		return nil, err
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
		return nil, err
	}
	return &server, nil
}

// CreateMCPServer persists a new MCP server record.
func CreateMCPServer(server *MCPServer) error {
	if server == nil {
		return errors.New("mcp server is nil")
	}
	if err := server.NormalizeAndValidate(); err != nil {
		return err
	}
	if err := encryptMCPServerSecret(server); err != nil {
		return err
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
		return err
	}
	if err := encryptMCPServerSecret(server); err != nil {
		return err
	}
	if err := DB.Model(server).Updates(server).Error; err != nil {
		return errors.Wrap(err, "update mcp server")
	}
	return nil
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
		return nil, err
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
		return nil, err
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
			return err
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
