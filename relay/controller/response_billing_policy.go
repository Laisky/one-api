package controller

import (
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/tooling"
)

// validateNativeResponseToolBilling admits only local tools and hosted tools
// with an implemented native Responses billing path. Unknown tool types must
// not silently acquire paid upstream execution when a provider adds a feature.
func validateNativeResponseToolBilling(tools []openai.ResponseAPITool) error {
	for _, tool := range tools {
		kind := strings.ToLower(strings.TrimSpace(tool.Type))
		if kind == "function" || kind == "custom" || (kind == "" && tool.Function != nil) {
			continue
		}
		if tooling.NormalizeBuiltinType(kind) == "web_search" {
			continue
		}
		return errors.Errorf("native Responses tool %q has no supported billing path", tool.Type)
	}
	return nil
}

// nativeResponseRootAllowed is an explicit protocol admission manifest. New
// typed fields require review instead of automatically enabling paid features.
// Provider extensions still use the shared controlled extra_body policy.
func nativeResponseRootAllowed(key string) bool {
	switch key {
	case "thinking", "output_config", "input", "model", "extra_body",
		"background", "conversation", "include", "instructions", "max_output_tokens",
		"metadata", "parallel_tool_calls", "previous_response_id", "prompt",
		"reasoning", "service_tier", "store", "stream", "temperature", "text",
		"tool_choice", "tools", "top_p", "truncation", "user":
		return true
	default:
		return isAllowedExtraBodyKey(key)
	}
}
