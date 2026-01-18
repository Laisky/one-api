package model

import "strings"

// MCPTool stores tool metadata synchronized from MCP servers.
type MCPTool struct {
	Id             int                  `json:"id"`
	ServerId       int                  `json:"server_id" gorm:"index"`
	Name           string               `json:"name" gorm:"type:varchar(128);index"`
	DisplayName    string               `json:"display_name" gorm:"type:varchar(128)"`
	Description    string               `json:"description" gorm:"type:text"`
	InputSchema    string               `json:"input_schema" gorm:"type:text"`
	DefaultPricing ToolPricingLocalJSON `json:"default_pricing" gorm:"type:text"`
	Status         int                  `json:"status" gorm:"type:int;default:1"`
	CreatedAt      int64                `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt      int64                `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

// NormalizeName ensures tool names are normalized consistently.
func (t *MCPTool) NormalizeName() {
	if t == nil {
		return
	}
	t.Name = strings.TrimSpace(strings.ToLower(t.Name))
}
