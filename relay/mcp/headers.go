package mcp

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
)

const (
	mcpBase64SentinelPrefix = "=?base64?"
	mcpBase64SentinelSuffix = "?="
	maxMCPHeaderInteger     = int64(1<<53 - 1)
)

type toolHeaderBinding struct {
	HeaderName string
	Path       []string
	ValueType  string
}

// ToolDescriptorRejection describes a tool excluded because its x-mcp-header schema is invalid.
type ToolDescriptorRejection struct {
	Name string
	Err  error
}

// ToolArgumentHeaders derives MCP-Param-* headers from x-mcp-header annotations in a tool schema.
func ToolArgumentHeaders(schema map[string]any, arguments map[string]any) (http.Header, error) {
	bindings, err := collectToolHeaderBindings(schema)
	if err != nil {
		return nil, errors.Wrap(err, "collect mcp tool header bindings")
	}
	headers := make(http.Header, len(bindings))
	for _, binding := range bindings {
		value, ok := lookupToolArgument(arguments, binding.Path)
		if !ok || value == nil {
			continue
		}
		encoded, err := formatToolHeaderValue(value, binding.ValueType)
		if err != nil {
			return nil, errors.Wrapf(err, "format mcp tool header %s", binding.HeaderName)
		}
		headers.Set(ParameterHeaderPrefix+binding.HeaderName, encoded)
	}
	return headers, nil
}

// ValidateToolArgumentHeaders verifies that mirrored MCP parameter headers match the JSON arguments.
func ValidateToolArgumentHeaders(requestHeaders http.Header, schema map[string]any, arguments map[string]any) error {
	expected, err := ToolArgumentHeaders(schema, arguments)
	if err != nil {
		return errors.Wrap(err, "derive expected mcp tool headers")
	}
	actual := make(http.Header)
	for key, values := range requestHeaders {
		if !strings.HasPrefix(strings.ToLower(key), strings.ToLower(ParameterHeaderPrefix)) {
			continue
		}
		actual[key] = append([]string(nil), values...)
	}
	if len(actual) != len(expected) {
		return errors.Errorf("mcp parameter header count mismatch: expected %d, got %d", len(expected), len(actual))
	}
	for key, values := range expected {
		if len(values) != 1 {
			return errors.Errorf("mcp parameter header %s has invalid expected cardinality", key)
		}
		actualValues := actual.Values(key)
		if len(actualValues) != 1 {
			return errors.Errorf("mcp parameter header %s must occur exactly once", key)
		}
		want, err := DecodeMCPHeaderValue(values[0])
		if err != nil {
			return errors.Wrapf(err, "decode expected mcp parameter header %s", key)
		}
		got, err := DecodeMCPHeaderValue(actualValues[0])
		if err != nil {
			return errors.Wrapf(err, "decode mcp parameter header %s", key)
		}
		if got != want {
			return errors.Errorf("mcp parameter header %s mismatch", key)
		}
	}
	return nil
}

// ValidateToolSchemaHeaders validates x-mcp-header annotations without requiring argument values.
func ValidateToolSchemaHeaders(schema map[string]any) error {
	_, err := collectToolHeaderBindings(schema)
	if err != nil {
		return errors.Wrap(err, "validate mcp tool schema headers")
	}
	return nil
}

// ValidateToolDescriptor validates required tool fields and HTTP header annotations.
func ValidateToolDescriptor(tool ToolDescriptor) error {
	if strings.TrimSpace(tool.Name) == "" {
		return errors.New("mcp tool name is required")
	}
	if tool.InputSchema == nil {
		return errors.New("mcp tool inputSchema is required")
	}
	if err := ValidateToolSchemaHeaders(tool.InputSchema); err != nil {
		return errors.Wrap(err, "validate mcp tool inputSchema")
	}
	return nil
}

// FilterValidToolDescriptors excludes tools whose x-mcp-header annotations violate the HTTP transport rules.
func FilterValidToolDescriptors(tools []ToolDescriptor) ([]ToolDescriptor, []ToolDescriptorRejection) {
	valid := make([]ToolDescriptor, 0, len(tools))
	rejected := make([]ToolDescriptorRejection, 0)
	for _, tool := range tools {
		if err := ValidateToolDescriptor(tool); err != nil {
			rejected = append(rejected, ToolDescriptorRejection{Name: tool.Name, Err: err})
			continue
		}
		valid = append(valid, tool)
	}
	return valid, rejected
}

// EncodeMCPHeaderValue applies the protocol's Base64 sentinel to unsafe HTTP header values.
func EncodeMCPHeaderValue(value string) string {
	if headerValueRequiresEncoding(value) {
		return mcpBase64SentinelPrefix + base64.StdEncoding.EncodeToString([]byte(value)) + mcpBase64SentinelSuffix
	}
	return value
}

// DecodeMCPHeaderValue decodes the protocol's Base64 sentinel or validates a plain HTTP header value.
func DecodeMCPHeaderValue(value string) (string, error) {
	if strings.HasPrefix(value, mcpBase64SentinelPrefix) || strings.HasSuffix(value, mcpBase64SentinelSuffix) {
		if !strings.HasPrefix(value, mcpBase64SentinelPrefix) || !strings.HasSuffix(value, mcpBase64SentinelSuffix) {
			return "", errors.New("malformed mcp base64 header sentinel")
		}
		encoded := strings.TrimSuffix(strings.TrimPrefix(value, mcpBase64SentinelPrefix), mcpBase64SentinelSuffix)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", errors.Wrap(err, "decode mcp base64 header value")
		}
		if !utf8.Valid(decoded) {
			return "", errors.New("decoded mcp header value is not valid UTF-8")
		}
		return string(decoded), nil
	}
	if headerValueRequiresEncoding(value) {
		return "", errors.New("unsafe plain mcp header value")
	}
	return value, nil
}

// collectToolHeaderBindings walks statically reachable object properties and returns header bindings.
func collectToolHeaderBindings(schema map[string]any) ([]toolHeaderBinding, error) {
	if len(schema) == 0 {
		return nil, nil
	}
	if err := validateToolHeaderAnnotationPlacement(schema, true, false, "$"); err != nil {
		return nil, errors.Wrap(err, "validate x-mcp-header placement")
	}
	bindings := make([]toolHeaderBinding, 0)
	seen := make(map[string]string)
	if err := walkToolSchemaProperties(schema, nil, &bindings, seen); err != nil {
		return nil, errors.Wrap(err, "walk mcp tool schema properties")
	}
	return bindings, nil
}

// validateToolHeaderAnnotationPlacement rejects annotations outside a properties-only path from the schema root.
func validateToolHeaderAnnotationPlacement(value any, reachable bool, isProperty bool, location string) error {
	switch typed := value.(type) {
	case map[string]any:
		if _, exists := typed["x-mcp-header"]; exists && (!reachable || !isProperty) {
			return errors.Errorf("x-mcp-header at %s is not statically reachable through properties", location)
		}
		for key, child := range typed {
			if key == "x-mcp-header" {
				continue
			}
			if key == "properties" && reachable {
				properties, ok := child.(map[string]any)
				if !ok {
					continue
				}
				for name, property := range properties {
					if err := validateToolHeaderAnnotationPlacement(property, true, true, location+".properties."+name); err != nil {
						return err
					}
				}
				continue
			}
			if err := validateToolHeaderAnnotationPlacement(child, false, false, location+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := validateToolHeaderAnnotationPlacement(child, false, false, location+"["+strconv.Itoa(index)+"]"); err != nil {
				return err
			}
		}
	}
	return nil
}

// walkToolSchemaProperties recursively visits object properties that have deterministic argument paths.
func walkToolSchemaProperties(schema map[string]any, path []string, bindings *[]toolHeaderBinding, seen map[string]string) error {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	for propertyName, rawProperty := range properties {
		propertySchema, ok := rawProperty.(map[string]any)
		if !ok {
			continue
		}
		propertyPath := append(append([]string(nil), path...), propertyName)
		if annotation, ok := propertySchema["x-mcp-header"]; ok {
			headerName, ok := annotation.(string)
			if !ok || headerName == "" {
				return errors.Errorf("x-mcp-header at %s must be a non-empty string", strings.Join(propertyPath, "."))
			}
			if !isValidMCPHeaderToken(headerName) {
				return errors.Errorf("x-mcp-header %q at %s is not a valid header token", headerName, strings.Join(propertyPath, "."))
			}
			valueType, _ := propertySchema["type"].(string)
			if valueType != "string" && valueType != "integer" && valueType != "boolean" {
				return errors.Errorf("x-mcp-header %q at %s requires string, integer, or boolean type", headerName, strings.Join(propertyPath, "."))
			}
			canonical := strings.ToLower(headerName)
			if previous, exists := seen[canonical]; exists {
				return errors.Errorf("x-mcp-header %q is duplicated at %s and %s", headerName, previous, strings.Join(propertyPath, "."))
			}
			seen[canonical] = strings.Join(propertyPath, ".")
			*bindings = append(*bindings, toolHeaderBinding{HeaderName: headerName, Path: propertyPath, ValueType: valueType})
		}
		if _, hasNestedProperties := propertySchema["properties"]; hasNestedProperties {
			if err := walkToolSchemaProperties(propertySchema, propertyPath, bindings, seen); err != nil {
				return err
			}
		}
	}
	return nil
}

// lookupToolArgument resolves a deterministic property path from JSON-like arguments.
func lookupToolArgument(arguments map[string]any, path []string) (any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	var current any = arguments
	for _, segment := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// formatToolHeaderValue converts a primitive JSON value into its MCP header representation.
func formatToolHeaderValue(value any, valueType string) (string, error) {
	var rendered string
	switch valueType {
	case "string":
		text, ok := value.(string)
		if !ok {
			return "", errors.Errorf("expected string, got %T", value)
		}
		rendered = text
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return "", errors.Errorf("expected boolean, got %T", value)
		}
		rendered = strconv.FormatBool(boolean)
	case "integer":
		integer, err := renderInteger(value)
		if err != nil {
			return "", err
		}
		rendered = integer
	default:
		return "", errors.Errorf("unsupported mcp header value type %q", valueType)
	}
	return EncodeMCPHeaderValue(rendered), nil
}

// renderInteger formats JavaScript-safe integer representations without losing precision.
func renderInteger(value any) (string, error) {
	var signed int64
	var unsigned uint64
	var isUnsigned bool
	switch typed := value.(type) {
	case int:
		signed = int64(typed)
	case int8:
		signed = int64(typed)
	case int16:
		signed = int64(typed)
	case int32:
		signed = int64(typed)
	case int64:
		signed = typed
	case uint:
		unsigned, isUnsigned = uint64(typed), true
	case uint8:
		unsigned, isUnsigned = uint64(typed), true
	case uint16:
		unsigned, isUnsigned = uint64(typed), true
	case uint32:
		unsigned, isUnsigned = uint64(typed), true
	case uint64:
		unsigned, isUnsigned = typed, true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || math.Abs(typed) > float64(maxMCPHeaderInteger) {
			return "", errors.Errorf("expected JavaScript-safe integer, got %v", typed)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case json.Number:
		integer, err := typed.Int64()
		if err != nil {
			return "", errors.Wrap(err, "parse json integer")
		}
		signed = integer
	default:
		return "", errors.Errorf("expected integer, got %T", value)
	}
	if isUnsigned {
		if unsigned > uint64(maxMCPHeaderInteger) {
			return "", errors.Errorf("integer %d exceeds the JavaScript-safe range", unsigned)
		}
		return strconv.FormatUint(unsigned, 10), nil
	}
	if signed < -maxMCPHeaderInteger || signed > maxMCPHeaderInteger {
		return "", errors.Errorf("integer %d exceeds the JavaScript-safe range", signed)
	}
	return strconv.FormatInt(signed, 10), nil
}

// headerValueRequiresEncoding reports whether a value requires the MCP Base64 sentinel.
func headerValueRequiresEncoding(value string) bool {
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return true
	}
	if strings.HasPrefix(value, mcpBase64SentinelPrefix) && strings.HasSuffix(value, mcpBase64SentinelSuffix) {
		return true
	}
	for _, r := range value {
		if r == '\t' || r == ' ' || (r >= 0x21 && r <= 0x7e) {
			continue
		}
		return true
	}
	return false
}

// isValidMCPHeaderToken reports whether a schema annotation is safe in an HTTP header name.
func isValidMCPHeaderToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}
