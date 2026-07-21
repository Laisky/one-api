package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/mcp"
)

// MCPServerUpsertRequest describes MCP server create or update payloads.
type MCPServerUpsertRequest struct {
	Name                    *string                           `json:"name"`
	Description             *string                           `json:"description"`
	Status                  *int                              `json:"status"`
	Priority                *int64                            `json:"priority"`
	BaseURL                 *string                           `json:"base_url"`
	Protocol                *string                           `json:"protocol"`
	AuthType                *string                           `json:"auth_type"`
	APIKey                  *string                           `json:"api_key"`
	Headers                 map[string]string                 `json:"headers"`
	ToolWhitelist           []string                          `json:"tool_whitelist"`
	ToolBlacklist           []string                          `json:"tool_blacklist"`
	ToolPricing             map[string]model.ToolPricingLocal `json:"tool_pricing"`
	AutoSyncEnabled         *bool                             `json:"auto_sync_enabled"`
	AutoSyncIntervalMinutes *int                              `json:"auto_sync_interval_minutes"`
}

// GetMCPServers lists MCP servers with pagination.
func GetMCPServers(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}

	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Optional keyword search. "keyword" matches the other list endpoints; "q" is
	// accepted as an alias so a caller cannot silently get an unfiltered page.
	keyword := c.Query("keyword")
	if keyword == "" {
		keyword = c.Query("q")
	}

	servers, err := model.ListMCPServers(keyword, p*size, size, sortBy, sortOrder)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	filtered := make([]gin.H, 0, len(servers))
	for _, server := range servers {
		count, err := model.CountMCPTools(server.Id, nil)
		if err != nil {
			count = 0
		}
		filtered = append(filtered, gin.H{
			"server":     sanitizeMCPServer(server),
			"tool_count": count,
		})
	}

	total, err := model.CountMCPServers(keyword)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    filtered,
		"total":   total,
	})
}

// GetMCPServer returns details for a MCP server.
func GetMCPServer(c *gin.Context) {
	id, err := resolveMCPServerRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	server, err := model.GetMCPServerByID(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    sanitizeMCPServer(server),
	})
}

// CreateMCPServer creates a new MCP server.
func CreateMCPServer(c *gin.Context) {
	logger := gmw.GetLogger(c)
	var payload MCPServerUpsertRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&payload); err != nil {
		// Unparsable request body.
		helper.RespondError(c, errkind.InvalidRequestErr(errors.Wrap(err, "decode mcp server")))
		return
	}

	server := &model.MCPServer{}
	applyMCPServerPayload(server, payload)
	if err := server.NormalizeAndValidate(); err != nil {
		// Payload failed the entity's own validation rules.
		helper.RespondError(c, errkind.InvalidRequestErr(errors.Wrap(err, "normalize and validate mcp server")))
		return
	}
	nameTaken, err := isMCPServerNameAlreadyUsed(server.Name, 0)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	if nameTaken {
		respondMCPServerNameTaken(c)
		return
	}
	if err := model.CreateMCPServer(server); err != nil {
		if isMCPServerNameTakenError(err) {
			respondMCPServerNameTaken(c)
			return
		}
		logger.Error("failed to create mcp server", append(server.Ref().Zap(), zap.Error(err))...)
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    sanitizeMCPServer(server),
	})
}

// UpdateMCPServer updates an existing MCP server.
func UpdateMCPServer(c *gin.Context) {
	logger := gmw.GetLogger(c)
	id, err := resolveMCPServerRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	payload, providedFields, err := bindMCPServerPayload(c)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	server, err := model.GetMCPServerByID(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	applyMCPServerPayload(server, payload)
	// Mask handling: when the api_key was present in the payload but the value
	// is a masked secret placeholder, we leave the existing value untouched
	// and must NOT overwrite the column with an empty string.
	if payload.APIKey != nil && common.IsMaskedSecret(*payload.APIKey) {
		delete(providedFields, "api_key")
	}
	if err := server.NormalizeAndValidate(); err != nil {
		// Payload failed the entity's own validation rules.
		helper.RespondError(c, errkind.InvalidRequestErr(errors.Wrap(err, "normalize and validate mcp server")))
		return
	}
	if providedFields["name"] {
		nameTaken, err := isMCPServerNameAlreadyUsed(server.Name, server.Id)
		if err != nil {
			helper.RespondError(c, err)
			return
		}
		if nameTaken {
			respondMCPServerNameTaken(c)
			return
		}
	}
	server.ProvidedFields = providedFields
	if err := model.UpdateMCPServer(server); err != nil {
		if isMCPServerNameTakenError(err) {
			respondMCPServerNameTaken(c)
			return
		}
		logger.Error("failed to update mcp server", append(server.Ref().Zap(), zap.Error(err))...)
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    sanitizeMCPServer(server),
	})
}

// bindMCPServerPayload decodes the upsert request and returns a map of which
// database columns were explicitly present in the raw request body. Mirrors
// bindChannelPayload so we can persist zero/empty values that GORM's
// struct-based Updates would otherwise silently skip.
func bindMCPServerPayload(c *gin.Context) (MCPServerUpsertRequest, map[string]bool, error) {
	var payload MCPServerUpsertRequest
	if err := common.UnmarshalBodyReusable(c, &payload); err != nil {
		return payload, nil, errors.Wrap(err, "unmarshal mcp server payload")
	}

	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		return payload, nil, errors.Wrap(err, "get request body")
	}

	rawFields := make(map[string]json.RawMessage)
	if len(requestBody) > 0 {
		if err := json.Unmarshal(requestBody, &rawFields); err != nil {
			return payload, nil, errors.Wrap(err, "unmarshal raw mcp server fields")
		}
	}

	// Map JSON field names to database column names. Only include columns
	// that the UpdateMCPServer store path knows how to apply per-column.
	jsonToColumn := map[string]string{
		"name":                       "name",
		"description":                "description",
		"status":                     "status",
		"priority":                   "priority",
		"base_url":                   "base_url",
		"protocol":                   "protocol",
		"auth_type":                  "auth_type",
		"api_key":                    "api_key",
		"headers":                    "headers",
		"tool_whitelist":             "tool_whitelist",
		"tool_blacklist":             "tool_blacklist",
		"tool_pricing":               "tool_pricing",
		"auto_sync_enabled":          "auto_sync_enabled",
		"auto_sync_interval_minutes": "auto_sync_interval_minutes",
	}
	provided := make(map[string]bool, len(jsonToColumn))
	for jsonName, column := range jsonToColumn {
		if _, ok := rawFields[jsonName]; ok {
			provided[column] = true
		}
	}
	return payload, provided, nil
}

// DeleteMCPServer deletes a MCP server by ID.
func DeleteMCPServer(c *gin.Context) {
	id, err := resolveMCPServerRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	if err := model.DeleteMCPServer(id); err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// SyncMCPServer triggers a manual tool sync for a MCP server.
func SyncMCPServer(c *gin.Context) {
	id, err := resolveMCPServerRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	server, err := model.GetMCPServerByID(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	count, err := mcp.SyncServerTools(gmw.Ctx(c), server)
	if err != nil {
		server.MarkSyncResult(false, err.Error())
		_ = model.UpdateMCPServer(server)
		helper.RespondError(c, err)
		return
	}

	server.MarkSyncResult(true, "")
	if err := model.UpdateMCPServer(server); err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"tool_count": count,
		},
	})
}

// TestMCPServer validates connectivity with a MCP server.
func TestMCPServer(c *gin.Context) {
	id, err := resolveMCPServerRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	server, err := model.GetMCPServerByID(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	client := mcp.NewStreamableHTTPClient(server, nil, 15*time.Second)
	tools, err := client.ListTools(gmw.Ctx(c))
	if err != nil {
		server.MarkTestResult(false, err.Error())
		_ = model.UpdateMCPServer(server)
		helper.RespondError(c, err)
		return
	}

	server.MarkTestResult(true, "")
	if err := model.UpdateMCPServer(server); err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"tool_count": len(tools),
			"protocol":   server.Protocol,
		},
	})
}

// ListMCPServerTools returns tools for a MCP server.
func ListMCPServerTools(c *gin.Context) {
	logger := gmw.GetLogger(c)
	id, err := resolveMCPServerRef(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	server, err := model.GetMCPServerByID(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	tools, err := model.GetMCPToolsByServerID(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	matched := applyMCPToolPricingToTools(tools, server.ToolPricing)
	normalizedSchemas := normalizeMCPToolInputSchemas(tools)
	if logger != nil && len(server.ToolPricing) > 0 {
		logger.Debug("mcp tool pricing applied", server.Ref().AppendZap([]zap.Field{
			zap.Int("pricing_entries", len(server.ToolPricing)),
			zap.Int("tool_count", len(tools)),
			zap.Int("matched", matched),
		})...)
	}
	if logger != nil && normalizedSchemas > 0 {
		logger.Debug("mcp tool schema normalized", append(server.Ref().Zap(), zap.Int("normalized", normalizedSchemas))...)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    tools,
	})
}

// ToolsDisplayServerEntry represents a MCP server with its tools for the public display page.
type ToolsDisplayServerEntry struct {
	Server *MCPServerDisplayInfo `json:"server"`
	Tools  []*model.MCPTool      `json:"tools"`
}

// MCPServerDisplayInfo is a sanitized view of MCPServer for public display (no secrets).
type MCPServerDisplayInfo struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Status   int    `json:"status"`
	Protocol string `json:"protocol"`
}

// GetToolsDisplay returns all enabled MCP servers and their enabled tools for the public tools page.
// Anonymous users see all enabled tools; logged-in users see the same (no per-user tool filtering yet).
func GetToolsDisplay(c *gin.Context) {
	servers, err := model.ListEnabledMCPServers()
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "Failed to load MCP servers"))
		return
	}

	result := make([]ToolsDisplayServerEntry, 0, len(servers))
	for _, server := range servers {
		tools, err := model.GetMCPToolsByServerID(server.Id)
		if err != nil {
			continue
		}

		// Apply server-level pricing overrides and normalize schemas
		applyMCPToolPricingToTools(tools, server.ToolPricing)
		normalizeMCPToolInputSchemas(tools)

		// Filter to enabled tools only
		enabledTools := make([]*model.MCPTool, 0, len(tools))
		for _, tool := range tools {
			if tool != nil && tool.Status == 1 {
				enabledTools = append(enabledTools, tool)
			}
		}

		if len(enabledTools) == 0 {
			continue
		}

		result = append(result, ToolsDisplayServerEntry{
			Server: &MCPServerDisplayInfo{
				UUID:     server.UUID,
				Name:     server.Name,
				Status:   server.Status,
				Protocol: server.Protocol,
			},
			Tools: enabledTools,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
}

// applyMCPToolPricingToTools applies MCP server pricing to tool records for response rendering.
func applyMCPToolPricingToTools(tools []*model.MCPTool, pricing map[string]model.ToolPricingLocal) int {
	if len(tools) == 0 || len(pricing) == 0 {
		return 0
	}
	normalized := make(map[string]model.ToolPricingLocal, len(pricing))
	for name, entry := range pricing {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		normalized[strings.ToLower(trimmed)] = entry
	}
	if len(normalized) == 0 {
		return 0
	}
	matched := 0
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		toolName := strings.TrimSpace(tool.Name)
		if toolName == "" {
			continue
		}
		if entry, ok := normalized[strings.ToLower(toolName)]; ok {
			tool.DefaultPricing = model.ToolPricingLocalJSON(entry)
			matched++
		}
	}
	return matched
}

// normalizeMCPToolInputSchemas replaces serialized null schema strings with empty values.
func normalizeMCPToolInputSchemas(tools []*model.MCPTool) int {
	if len(tools) == 0 {
		return 0
	}
	count := 0
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		trimmed := strings.TrimSpace(tool.InputSchema)
		if trimmed == "" {
			continue
		}
		if strings.EqualFold(trimmed, "null") {
			tool.InputSchema = ""
			count++
		}
	}
	return count
}

// applyMCPServerPayload copies request fields into the MCP server model.
func applyMCPServerPayload(server *model.MCPServer, payload MCPServerUpsertRequest) {
	if payload.Name != nil {
		server.Name = *payload.Name
	}
	if payload.Description != nil {
		server.Description = *payload.Description
	}
	if payload.Status != nil {
		server.Status = *payload.Status
	}
	if payload.Priority != nil {
		server.Priority = *payload.Priority
	}
	if payload.BaseURL != nil {
		server.BaseURL = *payload.BaseURL
	}
	if payload.Protocol != nil {
		server.Protocol = *payload.Protocol
	}
	if payload.AuthType != nil {
		server.AuthType = *payload.AuthType
	}
	if payload.APIKey != nil {
		if !common.IsMaskedSecret(*payload.APIKey) {
			server.APIKey = *payload.APIKey
		}
	}
	if payload.Headers != nil {
		server.Headers = payload.Headers
	}
	if payload.ToolWhitelist != nil {
		server.ToolWhitelist = payload.ToolWhitelist
	}
	if payload.ToolBlacklist != nil {
		server.ToolBlacklist = payload.ToolBlacklist
	}
	if payload.ToolPricing != nil {
		server.ToolPricing = payload.ToolPricing
	}
	if payload.AutoSyncEnabled != nil {
		server.AutoSyncEnabled = *payload.AutoSyncEnabled
	}
	if payload.AutoSyncIntervalMinutes != nil {
		server.AutoSyncIntervalMinutes = *payload.AutoSyncIntervalMinutes
	}
}

// isMCPServerNameAlreadyUsed reports whether another MCP server already owns
// the normalized name. excludedID lets update requests ignore the current row.
func isMCPServerNameAlreadyUsed(name string, excludedID int) (bool, error) {
	query := model.DB.Model(&model.MCPServer{}).Where("name = ?", strings.TrimSpace(name))
	if excludedID > 0 {
		query = query.Where("id <> ?", excludedID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, errors.Wrap(err, "check mcp server name")
	}
	return count > 0, nil
}

// isMCPServerNameTakenError reports whether an MCP server write failed because
// the database rejected a duplicate unique name.
func isMCPServerNameTakenError(err error) bool {
	return isDuplicateDBErrorForField(err, "name", "mcp_servers.name", "idx_mcp_servers_name")
}

// respondMCPServerNameTaken returns a public duplicate-name response for MCP
// server create and update requests.
func respondMCPServerNameTaken(c *gin.Context) {
	respondDuplicateOperation(c, "MCP server name already exists")
}

func sanitizeMCPServer(server *model.MCPServer) *model.MCPServer {
	if server == nil {
		return nil
	}
	copy := *server
	copy.APIKey = common.MaskSecret(copy.APIKey)
	return &copy
}
