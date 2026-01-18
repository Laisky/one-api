package model

import (
	"net/url"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
)

const (
	// MCPServerStatusDisabled marks a server as disabled.
	MCPServerStatusDisabled = 0
	// MCPServerStatusEnabled marks a server as enabled.
	MCPServerStatusEnabled = 1
)

const (
	// MCPProtocolStreamableHTTP indicates Streamable HTTP MCP.
	MCPProtocolStreamableHTTP = "streamable_http"
)

const (
	// MCPAuthTypeNone indicates no authentication for MCP server.
	MCPAuthTypeNone = "none"
	// MCPAuthTypeBearer indicates bearer token auth.
	MCPAuthTypeBearer = "bearer"
	// MCPAuthTypeAPIKey indicates API key auth.
	MCPAuthTypeAPIKey = "api_key"
	// MCPAuthTypeCustomHeaders indicates custom headers auth.
	MCPAuthTypeCustomHeaders = "custom_headers"
)

// MCPServer stores admin-managed MCP server metadata and policies.
type MCPServer struct {
	Id                      int               `json:"id"`
	Name                    string            `json:"name" gorm:"uniqueIndex;type:varchar(128);not null"`
	Description             string            `json:"description" gorm:"type:text"`
	Status                  int               `json:"status" gorm:"type:int;default:1"`
	Priority                int64             `json:"priority" gorm:"bigint;default:0"`
	BaseURL                 string            `json:"base_url" gorm:"type:text;not null"`
	Protocol                string            `json:"protocol" gorm:"type:varchar(32);default:'streamable_http'"`
	AuthType                string            `json:"auth_type" gorm:"type:varchar(32);default:'none'"`
	APIKey                  string            `json:"api_key" gorm:"type:text"`
	Headers                 JSONStringMap     `json:"headers" gorm:"type:text"`
	ToolWhitelist           JSONStringSlice   `json:"tool_whitelist" gorm:"type:text"`
	ToolBlacklist           JSONStringSlice   `json:"tool_blacklist" gorm:"type:text"`
	ToolPricing             MCPToolPricingMap `json:"tool_pricing" gorm:"type:text"`
	AutoSyncEnabled         bool              `json:"auto_sync_enabled" gorm:"default:true"`
	AutoSyncIntervalMinutes int               `json:"auto_sync_interval_minutes" gorm:"column:auto_sync_interval_minutes;default:60"`
	LastSyncAt              int64             `json:"last_sync_at" gorm:"bigint"`
	LastSyncStatus          string            `json:"last_sync_status" gorm:"type:varchar(32);default:''"`
	LastSyncError           string            `json:"last_sync_error" gorm:"type:text"`
	LastTestAt              int64             `json:"last_test_at" gorm:"bigint"`
	LastTestStatus          string            `json:"last_test_status" gorm:"type:varchar(32);default:''"`
	LastTestError           string            `json:"last_test_error" gorm:"type:text"`
	CreatedAt               int64             `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt               int64             `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

// GetPriority returns the configured MCP server priority.
func (s *MCPServer) GetPriority() int64 {
	if s == nil {
		return 0
	}
	return s.Priority
}

// NormalizeAndValidate ensures MCP server fields conform to policy expectations.
func (s *MCPServer) NormalizeAndValidate() error {
	if s == nil {
		return errors.New("mcp server is nil")
	}

	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return errors.New("mcp server name is required")
	}

	s.BaseURL = strings.TrimSpace(s.BaseURL)
	if s.BaseURL == "" {
		return errors.New("mcp server base_url is required")
	}
	parsedURL, err := url.Parse(s.BaseURL)
	if err != nil {
		return errors.Wrap(err, "invalid mcp server base_url")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("mcp server base_url must use http or https")
	}

	s.Protocol = strings.TrimSpace(strings.ToLower(s.Protocol))
	if s.Protocol == "" {
		s.Protocol = MCPProtocolStreamableHTTP
	}

	s.AuthType = strings.TrimSpace(strings.ToLower(s.AuthType))
	if s.AuthType == "" {
		s.AuthType = MCPAuthTypeNone
	}

	if s.AutoSyncIntervalMinutes == 0 {
		s.AutoSyncIntervalMinutes = 60
	}

	if s.AutoSyncIntervalMinutes < 5 || s.AutoSyncIntervalMinutes > 1440 {
		return errors.New("auto_sync_interval_minutes must be between 5 and 1440")
	}

	if err := s.ValidateToolPricing(); err != nil {
		return err
	}

	return nil
}

// ValidateToolPricing ensures per-tool pricing values are non-negative.
func (s *MCPServer) ValidateToolPricing() error {
	for name, pricing := range s.ToolPricing {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return errors.New("tool pricing contains empty tool name")
		}
		if pricing.UsdPerCall < 0 {
			return errors.Errorf("tool %s usd_per_call cannot be negative", trimmed)
		}
		if pricing.QuotaPerCall < 0 {
			return errors.Errorf("tool %s quota_per_call cannot be negative", trimmed)
		}
	}
	return nil
}

// MarkSyncResult updates sync metadata based on the latest attempt.
func (s *MCPServer) MarkSyncResult(ok bool, errMsg string) {
	if s == nil {
		return
	}
	now := time.Now().UTC().UnixMilli()
	s.LastSyncAt = now
	if ok {
		s.LastSyncStatus = "ok"
		s.LastSyncError = ""
		return
	}
	s.LastSyncStatus = "error"
	s.LastSyncError = errMsg
}

// MarkTestResult updates test metadata based on the latest attempt.
func (s *MCPServer) MarkTestResult(ok bool, errMsg string) {
	if s == nil {
		return
	}
	now := time.Now().UTC().UnixMilli()
	s.LastTestAt = now
	if ok {
		s.LastTestStatus = "ok"
		s.LastTestError = ""
		return
	}
	s.LastTestStatus = "error"
	s.LastTestError = errMsg
}
