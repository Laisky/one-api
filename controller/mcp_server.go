package controller

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/mcp"
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

	servers, err := model.ListMCPServers(p*size, size, sortBy, sortOrder)
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

	total, err := model.CountMCPServers()
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
	id, err := strconv.Atoi(c.Param("id"))
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
		helper.RespondError(c, errors.Wrap(err, "decode mcp server"))
		return
	}

	server := &model.MCPServer{}
	applyMCPServerPayload(server, payload)
	if err := model.CreateMCPServer(server); err != nil {
		logger.Error("failed to create mcp server", zap.Error(err))
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
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	var payload MCPServerUpsertRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&payload); err != nil {
		helper.RespondError(c, errors.Wrap(err, "decode mcp server"))
		return
	}

	server, err := model.GetMCPServerByID(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	applyMCPServerPayload(server, payload)
	if err := model.UpdateMCPServer(server); err != nil {
		logger.Error("failed to update mcp server", zap.Error(err))
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    sanitizeMCPServer(server),
	})
}

// DeleteMCPServer deletes a MCP server by ID.
func DeleteMCPServer(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
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
	id, err := strconv.Atoi(c.Param("id"))
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
	id, err := strconv.Atoi(c.Param("id"))
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
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	tools, err := model.GetMCPToolsByServerID(id)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    tools,
	})
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

func sanitizeMCPServer(server *model.MCPServer) *model.MCPServer {
	if server == nil {
		return nil
	}
	copy := *server
	copy.APIKey = common.MaskSecret(copy.APIKey)
	return &copy
}
