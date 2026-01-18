package model

import (
	"strings"

	"github.com/Laisky/errors/v2"
)

// MCPToolSortFields enumerates sortable columns for MCP tool lists.
var MCPToolSortFields = map[string]string{
	"id":         "id",
	"name":       "name",
	"status":     "status",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

// ListMCPTools returns MCP tools filtered by server id and status.
func ListMCPTools(serverID int, status *int, offset int, limit int, sortBy string, sortOrder string) ([]*MCPTool, error) {
	query := DB.Model(&MCPTool{})
	if serverID > 0 {
		query = query.Where("server_id = ?", serverID)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if sortBy == "" {
		sortBy = "id"
	}
	column, ok := MCPToolSortFields[strings.ToLower(sortBy)]
	if !ok {
		column = "id"
	}
	order := "desc"
	if strings.ToLower(sortOrder) == "asc" {
		order = "asc"
	}
	query = query.Order(column + " " + order)

	var tools []*MCPTool
	if err := query.Find(&tools).Error; err != nil {
		return nil, errors.Wrap(err, "list mcp tools")
	}
	return tools, nil
}

// CountMCPTools returns the total number of MCP tools matching filters.
func CountMCPTools(serverID int, status *int) (int64, error) {
	query := DB.Model(&MCPTool{})
	if serverID > 0 {
		query = query.Where("server_id = ?", serverID)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, errors.Wrap(err, "count mcp tools")
	}
	return count, nil
}

// GetMCPToolsByServerID fetches tools for a specific server.
func GetMCPToolsByServerID(serverID int) ([]*MCPTool, error) {
	if serverID <= 0 {
		return nil, errors.New("server id is invalid")
	}
	var tools []*MCPTool
	if err := DB.Where("server_id = ?", serverID).Find(&tools).Error; err != nil {
		return nil, errors.Wrap(err, "get mcp tools")
	}
	return tools, nil
}

// UpsertMCPTools replaces tools for a server with the provided list.
func UpsertMCPTools(serverID int, tools []*MCPTool) error {
	if serverID <= 0 {
		return errors.New("server id is invalid")
	}
	if err := DB.Where("server_id = ?", serverID).Delete(&MCPTool{}).Error; err != nil {
		return errors.Wrap(err, "clear mcp tools")
	}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		tool.ServerId = serverID
		tool.NormalizeName()
		if err := DB.Create(tool).Error; err != nil {
			return errors.Wrap(err, "create mcp tool")
		}
	}
	return nil
}
