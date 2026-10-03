package mcp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
)

// ToolExecutionUncertainError prevents replay when an attempted tool execution cannot safely be retried.
// It does not assert that the upstream performed a side effect; it preserves that uncertainty.
type ToolExecutionUncertainError struct{ Err error }

// Error returns the wrapped failure message without inventing an upstream protocol error code.
func (e *ToolExecutionUncertainError) Error() string { return e.Err.Error() }

// Unwrap returns the original failure for cancellation and protocol-error inspection.
func (e *ToolExecutionUncertainError) Unwrap() error { return e.Err }

// isSafeToolLegacyFallback accepts only an explicit protocol-era rejection before repeating tools/call.
// Catalog discovery keeps broader legacy detection because tools/list has no tool side effects.
func isSafeToolLegacyFallback(err error) bool {
	var uncertain *ToolExecutionUncertainError
	if errors.As(err, &uncertain) {
		return false
	}
	var failure *ProtocolError
	if !errors.As(err, &failure) || !IsModernFallbackCandidate(err) {
		return false
	}
	if AdvertisesSupportedLegacyVersion(failure) {
		return true
	}
	if failure.HTTPStatus != http.StatusBadRequest && failure.HTTPStatus != http.StatusOK {
		return false
	}
	if failure.Code != -32600 && failure.Code != -32002 {
		return false
	}
	message := strings.ToLower(failure.Message)
	return strings.Contains(message, "initializ") || strings.Contains(message, "session")
}

// validateModernMCPResult rejects unsupported result variants and malformed MRTR containers.
// Missing resultType remains complete for legacy interoperability; unknown extensions are not negotiated.
func validateModernMCPResult(raw json.RawMessage, method string) error {
	var result map[string]json.RawMessage
	if err := DecodeJSON(raw, &result); err != nil {
		return err
	}
	if result == nil {
		return errors.New("MCP result must be an object")
	}
	resultType := ResultTypeComplete
	if value, exists := result["resultType"]; exists {
		resultType = ""
		if err := DecodeJSON(value, &resultType); err != nil {
			return errors.Wrap(err, "decode MCP resultType")
		}
	}
	switch resultType {
	case ResultTypeComplete:
		return nil
	case ResultTypeInputRequired:
		if method != "tools/call" {
			return errors.New("MCP input_required is not supported for this method")
		}
		var requests map[string]json.RawMessage
		if err := DecodeJSON(result["inputRequests"], &requests); err != nil {
			return errors.Wrap(err, "decode MCP inputRequests")
		}
		if len(requests) == 0 {
			return errors.New("MCP input_required needs at least one input request")
		}
		for _, rawRequest := range requests {
			var request struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := DecodeJSON(rawRequest, &request); err != nil {
				return errors.Wrap(err, "decode MCP input request")
			}
			switch request.Method {
			case "elicitation/create", "sampling/createMessage", "roots/list":
			default:
				return errors.New("unsupported MCP input request method")
			}
		}
		return nil
	default:
		return errors.Errorf("unsupported MCP resultType %q", resultType)
	}
}

// boundedMCPHeaderNumber bounds exact-number expansion before big.Rat sees an untrusted header argument.
// Header integers have the much narrower JavaScript-safe range; opaque non-header numbers are unaffected.
func boundedMCPHeaderNumber(number json.Number) bool {
	text := string(number)
	if len(text) > 128 {
		return false
	}
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		exponent, err := strconv.ParseInt(text[index+1:], 10, 32)
		return err == nil && exponent >= -128 && exponent <= 128
	}
	return true
}

// validateMCPToolInputCapabilities rejects unrequested server-to-client interactions before returning them to callers.
// An empty elicitation capability opts into form mode only; URL mode must be explicitly declared.
func validateMCPToolInputCapabilities(result *CallToolResult, meta map[string]any) error {
	if result == nil || result.ResultType != ResultTypeInputRequired {
		return nil
	}
	capabilities, _ := meta[MetaClientCapabilitiesKey].(map[string]any)
	for _, value := range result.InputRequests {
		request, _ := value.(map[string]any)
		method, _ := request["method"].(string)
		name := ""
		switch method {
		case "elicitation/create":
			name = "elicitation"
		case "sampling/createMessage":
			name = "sampling"
		case "roots/list":
			name = "roots"
		default:
			return errors.New("unsupported MCP input request method")
		}
		declared, ok := capabilities[name].(map[string]any)
		if !ok || declared == nil {
			return errors.Errorf("upstream requested undeclared MCP client capability %s", name)
		}
		if name != "elicitation" {
			continue
		}
		params, _ := request["params"].(map[string]any)
		mode := "form"
		if value, exists := params["mode"]; exists {
			mode, ok = value.(string)
			if !ok {
				return errors.New("upstream elicitation mode must be a string")
			}
		}
		if mode != "form" && mode != "url" {
			return errors.New("upstream elicitation mode is unsupported")
		}
		if mode == "form" && len(declared) == 0 {
			continue
		}
		if enabled, ok := declared[mode].(map[string]any); !ok || enabled == nil {
			return errors.Errorf("upstream requested undeclared elicitation mode %s", mode)
		}
	}
	return nil
}
