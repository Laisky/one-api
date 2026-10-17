package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// responseToolSearchInputItems obtains discovery history from typed input while
// preserving the original input object for native forwarding and state storage.
func responseToolSearchInputItems(input openai.ResponseAPIInput) ([]openai.ResponseToolSearchInputItem, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, errors.Wrap(err, "marshal Responses discovery history")
	}
	return openai.InspectResponseToolSearchInput(data)
}

// validateResponseToolSearchInput checks every dynamic catalog before routing,
// state resolution and reservation, including requests with no top-level tools.
func validateResponseToolSearchInput(input openai.ResponseAPIInput) error {
	items, err := responseToolSearchInputItems(input)
	if err != nil {
		return err
	}
	return validateResponseToolSearchItems(items)
}

// validateResponseToolSearchItems admits local discovery and local loaded tools.
// Server execution remains unsupported; existing hosted-tool policy is not
// extended to history catalogs without separate capability and receipt review.
func validateResponseToolSearchItems(items []openai.ResponseToolSearchInputItem) error {
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(item.Type)) {
		case "tool_search_call":
			arguments := bytes.TrimSpace(item.Arguments)
			if item.Execution != "client" || strings.TrimSpace(item.CallID) == "" || len(arguments) == 0 || arguments[0] != '{' {
				return errors.New("native Responses tool_search_call requires client execution, call_id and object arguments")
			}
		case "tool_search_output":
			if item.Execution != "client" || strings.TrimSpace(item.CallID) == "" || item.Tools == nil {
				return errors.New("native Responses tool_search_output requires client execution, call_id and tools array")
			}
			if err := validateResponseDiscoveredTools(item.Tools, false); err != nil {
				return err
			}
		case "additional_tools":
			if item.Role != "developer" || item.Tools == nil {
				return errors.New("native Responses additional_tools requires developer role and tools array")
			}
			if err := validateResponseDiscoveredTools(item.Tools, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateResponseDiscoveredTools admits a bounded local catalog without making
// hosted tools callable through a history item. A Lite developer catalog may
// also declare the same explicit client search tool as a standard request.
func validateResponseDiscoveredTools(tools []openai.ResponseAPITool, allowSearch bool) error {
	if len(tools) > openai.MaxResponseNamespaceTools {
		return errors.New("native Responses discovered tool count exceeds limit")
	}
	for _, tool := range tools {
		switch strings.ToLower(strings.TrimSpace(tool.Type)) {
		case "namespace":
			if err := validateLocalResponseNamespace(tool); err != nil {
				return err
			}
		case "function", "custom":
			name := tool.Name
			if tool.Function != nil && tool.Function.Name != "" {
				name = tool.Function.Name
			}
			if !validResponseNamespaceName(name) || len(tool.Tools) > 0 {
				return errors.New("native Responses discovered local tool requires a valid name and no children")
			}
		case "tool_search":
			if !allowSearch || tool.Execution != "client" || len(tool.Tools) > 0 {
				return errors.New("native Responses discovered tool_search has no supported local billing path")
			}
		default:
			return errors.Errorf("native Responses discovered tool %q has no supported local billing path", tool.Type)
		}
	}
	return nil
}

// responseToolSearchFallbackError rejects discovery definitions and history
// before fallback drops their semantics. It returns nil for ordinary input.
func responseToolSearchFallbackError(request *openai.ResponseAPIRequest) *relaymodel.ErrorWithStatusCode {
	for _, tool := range request.Tools {
		if strings.EqualFold(strings.TrimSpace(tool.Type), "tool_search") {
			return openai.ErrorWrapper(errors.New("tool search requires native Responses support"), "tool_search_not_supported", http.StatusBadRequest)
		}
	}
	items, err := responseToolSearchInputItems(request.Input)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_response_api_request", http.StatusBadRequest)
	}
	if len(items) > 0 {
		return openai.ErrorWrapper(errors.New("tool search history and catalogs require native Responses support"), "tool_search_not_supported", http.StatusBadRequest)
	}
	return nil
}
