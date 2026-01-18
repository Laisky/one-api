package dto

// UserAdminUpdatePayload describes optional fields for the admin user update endpoint.
// Pointer members let callers omit properties they do not wish to change while
// still allowing explicit zero values (such as empty strings or zero quotas).
// Only the user identifier is mandatory; all other attributes are optional.
type UserAdminUpdatePayload struct {
	Id               int       `json:"id"`
	Username         *string   `json:"username"`
	DisplayName      *string   `json:"display_name"`
	Password         *string   `json:"password"`
	Email            *string   `json:"email"`
	Quota            *int64    `json:"quota"`
	Group            *string   `json:"group"`
	Role             *int      `json:"role"`
	Status           *int      `json:"status"`
	MCPToolBlacklist *[]string `json:"mcp_tool_blacklist"`
}
