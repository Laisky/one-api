package model

import (
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// applyMCPToolKeyword narrows an MCP tool query by a search keyword.
//
// A pasted UUID matches the tool's own uuid or its owning server_uuid by equality, so an
// operator can paste either identifier copied out of the UI. Any other keyword falls back
// to a case-insensitive substring match over name/display_name. An empty keyword leaves
// the query untouched.
//
// Parameters:
//   - query: query to narrow; never mutated, GORM returns a new statement.
//   - keyword: raw search keyword supplied by the request.
//
// Return values:
//   - *gorm.DB: narrowed query, or query unchanged when the keyword is empty.
func applyMCPToolKeyword(query *gorm.DB, keyword string) *gorm.DB {
	trimmed := strings.TrimSpace(keyword)
	if trimmed == "" {
		return query
	}
	if scoped, matched := applyUUIDKeyword(query, trimmed, "uuid", "server_uuid"); matched {
		return scoped
	}
	pattern := "%" + strings.ToLower(trimmed) + "%"
	return query.Where("(LOWER(name) LIKE ? or LOWER(display_name) LIKE ?)", pattern, pattern)
}

// MCPToolSortFields enumerates whitelisted columns for MCP tool sorting.
var MCPToolSortFields = map[string]string{
	"id":         "id",
	"name":       "name",
	"status":     "status",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

// ListMCPTools returns MCP tools filtered by server, status, pagination, and ordering.
//
// Parameters:
//   - serverID: owning server id; non-positive means every server.
//   - status: optional status filter.
//   - offset: pagination offset.
//   - limit: page size; non-positive means unlimited.
//   - sortBy: whitelisted sort column.
//   - sortOrder: ascending or descending order.
//
// Return values:
//   - []*MCPTool: matching tools.
//   - error: a wrapped database error when the query fails.
func ListMCPTools(serverID int, status *int, offset int, limit int, sortBy string, sortOrder string) ([]*MCPTool, error) {
	return SearchMCPTools(serverID, status, "", offset, limit, sortBy, sortOrder)
}

// SearchMCPTools returns MCP tools filtered by server id, status, and a search keyword.
//
// Parameters:
//   - serverID: owning server id; non-positive means every server.
//   - status: optional status filter.
//   - keyword: optional search keyword; a UUID matches the tool uuid or its server_uuid
//     exactly, anything else substring-matches name/display_name. Empty means no filter.
//   - offset: pagination offset.
//   - limit: page size; non-positive means unlimited.
//   - sortBy: whitelisted sort column.
//   - sortOrder: "asc" or "desc".
//
// Return values:
//   - []*MCPTool: matching tools.
//   - error: wrapped database error when the query fails.
func SearchMCPTools(serverID int, status *int, keyword string, offset int, limit int, sortBy string, sortOrder string) ([]*MCPTool, error) {
	query := applyMCPToolKeyword(DB.Model(&MCPTool{}), keyword)
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

	orderClause := ValidateOrderClause(sortBy, sortOrder, MCPToolSortFields, "id desc")
	query = query.Order(orderClause)

	var tools []*MCPTool
	if err := query.Find(&tools).Error; err != nil {
		return nil, errors.Wrap(err, "list mcp tools")
	}
	return tools, nil
}

// CountMCPTools returns the total number of MCP tools matching server and status filters.
//
// Parameters:
//   - serverID: owning server id; non-positive means every server.
//   - status: optional status filter.
//
// Return values:
//   - int64: the number of matching tools.
//   - error: a wrapped database error when the count fails.
func CountMCPTools(serverID int, status *int) (int64, error) {
	return CountSearchedMCPTools(serverID, status, "")
}

// CountSearchedMCPTools returns the number of MCP tools matching filters and a keyword.
//
// It applies exactly the same keyword filter as SearchMCPTools so the reported total
// always agrees with the rows a client can page through.
//
// Parameters:
//   - serverID: owning server id; non-positive means every server.
//   - status: optional status filter.
//   - keyword: optional search keyword; empty means count every matching tool.
//
// Return values:
//   - int64: number of matching tools.
//   - error: wrapped database error when the query fails.
func CountSearchedMCPTools(serverID int, status *int, keyword string) (int64, error) {
	query := applyMCPToolKeyword(DB.Model(&MCPTool{}), keyword)
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
//
// Parameters:
//   - serverID: the positive internal id of the owning MCP server.
//
// Return values:
//   - []*MCPTool: a non-nil slice containing every stored tool for the server.
//   - error: a validation or wrapped database error.
func GetMCPToolsByServerID(serverID int) ([]*MCPTool, error) {
	if serverID <= 0 {
		return nil, errors.New("server id is invalid")
	}
	tools := make([]*MCPTool, 0)
	if err := DB.Where("server_id = ?", serverID).Find(&tools).Error; err != nil {
		return nil, errors.Wrap(err, "get mcp tools")
	}
	return tools, nil
}

// UpsertMCPTools atomically replaces a server catalog while preserving exact wire names and descriptors.
//
// Parameters:
//   - serverID: the positive internal id of the owning MCP server.
//   - serverUUID: the stable owning server UUID copied onto every tool row.
//   - tools: the complete replacement catalog; nil entries are ignored.
//
// Return values:
//   - error: a validation or wrapped transactional database error.
func UpsertMCPTools(serverID int, serverUUID string, tools []*MCPTool) error {
	if serverID <= 0 {
		return errors.New("server id is invalid")
	}
	if DB == nil {
		return errors.New("database is not initialized")
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("server_id = ?", serverID).Delete(&MCPTool{}).Error; err != nil {
			return errors.Wrap(err, "clear mcp tools")
		}
		for _, tool := range tools {
			if tool == nil {
				continue
			}
			tool.ServerId = serverID
			if serverUUID != "" {
				tool.ServerUUID = &serverUUID
			}
			tool.NormalizeName()
			if tool.Name == "" {
				return errors.New("mcp tool name is required")
			}
			if err := tx.Create(tool).Error; err != nil {
				return errors.Wrapf(err, "create mcp tool %q", tool.Name)
			}
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "replace mcp tool catalog")
	}
	return nil
}
