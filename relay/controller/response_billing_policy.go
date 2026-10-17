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
		if kind == "namespace" {
			if err := validateLocalResponseNamespace(tool); err != nil {
				return err
			}
			continue
		}
		if len(tool.Tools) > 0 {
			return errors.New("native Responses children require a namespace billing path")
		}

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

// nativeResponseRootDenied lists reviewed typed fields that are never forwarded
// to a provider. The parameter key is a root request key. It returns true for
// any spelling of "background": background execution has no durable terminal
// settlement (#483), so normalizeResponseAPIRawBody always strips the flag. It
// also returns true for "conversation": resolveNativeConversation diverts the
// owner's gateway conversation to the hydrating fallback and rejects every other
// selector, so the provider must never receive a conversation ID (only empty
// selectors reach the wire builder, and those name nothing).
func nativeResponseRootDenied(key string) bool {
	return openai.IsResponseBackgroundKey(key) || key == "conversation"
}

// nativeResponseRootAllowed is an explicit protocol admission manifest. New
// typed fields require review instead of automatically enabling paid features.
// Provider extensions still use the shared controlled extra_body policy.
// Reviewed but denied fields live in nativeResponseRootDenied instead.
func nativeResponseRootAllowed(key string) bool {
	switch key {
	case "thinking", "output_config", "input", "model", "extra_body",
		"include", "instructions", "max_output_tokens",
		"metadata", "parallel_tool_calls", "previous_response_id", "prompt",
		"reasoning", "service_tier", "store", "stream", "temperature", "text",
		"tool_choice", "tools", "top_p", "top_logprobs", "truncation", "user":
		// top_logprobs is a documented Responses field, handled by the final
		// model-parameter policy even though it is not represented in the DTO.
		return true
	default:
		return isAllowedExtraBodyKey(key)
	}
}

// validateLocalResponseNamespace admits a bounded group of named client-side
// function/custom definitions and returns a billing error for every other child.
func validateLocalResponseNamespace(group openai.ResponseAPITool) error {
	if !validResponseNamespaceName(group.Name) || len(group.Tools) == 0 || len(group.Tools) > openai.MaxResponseNamespaceTools {
		return errors.New("native Responses namespace billing requires a name and bounded nonempty local tools")
	}
	for _, child := range group.Tools {
		kind := strings.ToLower(strings.TrimSpace(child.Type))
		name := child.Name
		if child.Function != nil && child.Function.Name != "" {
			name = child.Function.Name
		}
		if (kind != "function" && kind != "custom") || !validResponseNamespaceName(name) || len(child.Tools) > 0 {
			return errors.Errorf("native Responses namespace child %q has no supported local billing path", child.Type)
		}
	}
	return nil
}

// validResponseNamespaceName checks the documented local tool identifier shape
// without changing historical flat-tool admission. It returns true for a short
// nonempty ASCII identifier containing letters, digits, underscores or hyphens.
func validResponseNamespaceName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}
