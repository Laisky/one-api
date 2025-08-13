package model

import (
	"testing"
)

// TestToolValidation tests the new validation methods
func TestToolValidation(t *testing.T) {
	tests := []struct {
		name    string
		tool    Tool
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid function tool",
			tool: Tool{
				Type: "function",
				Function: &Function{
					Name:        "test_function",
					Description: "Test function",
				},
			},
			wantErr: false,
		},
		{
			name: "Function tool with nil function",
			tool: Tool{
				Type:     "function",
				Function: nil,
			},
			wantErr: true,
			errMsg:  "function tool requires function definition",
		},
		{
			name: "Function tool with empty name",
			tool: Tool{
				Type: "function",
				Function: &Function{
					Name:        "",
					Description: "Test function",
				},
			},
			wantErr: true,
			errMsg:  "function name is required",
		},
		{
			name: "Valid MCP tool",
			tool: Tool{
				Type:        "mcp",
				ServerLabel: "test-server",
				ServerUrl:   "https://api.example.com/mcp",
			},
			wantErr: false,
		},
		{
			name: "MCP tool with missing server_label",
			tool: Tool{
				Type:      "mcp",
				ServerUrl: "https://api.example.com/mcp",
			},
			wantErr: true,
			errMsg:  "MCP tool requires server_label",
		},
		{
			name: "MCP tool with missing server_url",
			tool: Tool{
				Type:        "mcp",
				ServerLabel: "test-server",
			},
			wantErr: true,
			errMsg:  "MCP tool requires server_url",
		},
		{
			name: "MCP tool with invalid URL",
			tool: Tool{
				Type:        "mcp",
				ServerLabel: "test-server",
				ServerUrl:   "not-a-valid-url",
			},
			wantErr: true,
			errMsg:  "server_url must use http or https scheme",
		},
		{
			name: "MCP tool with invalid scheme",
			tool: Tool{
				Type:        "mcp",
				ServerLabel: "test-server",
				ServerUrl:   "ftp://api.example.com/mcp",
			},
			wantErr: true,
			errMsg:  "server_url must use http or https scheme",
		},
		{
			name: "Tool with unknown type but valid function",
			tool: Tool{
				Type: "unknown",
				Function: &Function{
					Name:        "test_function",
					Description: "Test function",
				},
			},
			wantErr: false, // Should default to function validation
		},
		{
			name: "Tool with unknown type and no function",
			tool: Tool{
				Type:     "unknown",
				Function: nil,
			},
			wantErr: false, // Should pass validation for unknown types with no function
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tool.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Expected error message '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestValidateFunction tests the ValidateFunction method specifically
func TestValidateFunction(t *testing.T) {
	tool := Tool{
		Type: "function",
		Function: &Function{
			Name:        "test_function",
			Description: "Test function",
		},
	}

	err := tool.ValidateFunction()
	if err != nil {
		t.Errorf("Expected no error for valid function tool, got: %v", err)
	}
}

// TestValidateMCP tests the ValidateMCP method specifically
func TestValidateMCP(t *testing.T) {
	tool := Tool{
		Type:        "mcp",
		ServerLabel: "test-server",
		ServerUrl:   "https://api.example.com/mcp",
	}

	err := tool.ValidateMCP()
	if err != nil {
		t.Errorf("Expected no error for valid MCP tool, got: %v", err)
	}
}
