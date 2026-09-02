package model

import "strings"

// MCPTool stores searchable MCP tool metadata and the complete upstream wire descriptor.
type MCPTool struct {
	Id             int                  `json:"-"`
	UUID           string               `json:"uuid" gorm:"type:char(36);column:uuid"`
	ServerId       int                  `json:"-" gorm:"index"`
	ServerUUID     *string              `json:"server_uuid" gorm:"type:char(36);column:server_uuid;index"`
	Name           string               `json:"name" gorm:"type:varchar(128);index"`
	DisplayName    string               `json:"display_name" gorm:"type:varchar(128)"`
	Description    string               `json:"description" gorm:"type:text"`
	InputSchema    string               `json:"input_schema" gorm:"type:text"`
	DescriptorJSON string               `json:"-" gorm:"type:text;column:descriptor_json"`
	DefaultPricing ToolPricingLocalJSON `json:"default_pricing" gorm:"type:text"`
	Status         int                  `json:"status" gorm:"type:int;default:1"`
	CreatedAt      int64                `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt      int64                `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

// NormalizeName trims transport-insignificant whitespace while preserving the case-sensitive wire name.
//
// Parameters: none.
//
// Return values: none; the receiver is updated in place when it is non-nil.
func (t *MCPTool) NormalizeName() {
	if t == nil {
		return
	}
	t.Name = strings.TrimSpace(t.Name)
}
