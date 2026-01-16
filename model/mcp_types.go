package model

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// JSONStringSlice stores a string slice as JSON in the database.
type JSONStringSlice []string

// Value converts the JSONStringSlice into a driver value.
func (s JSONStringSlice) Value() (driver.Value, error) {
	if len(s) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal([]string(s))
	if err != nil {
		return nil, errors.Wrap(err, "marshal json string slice")
	}
	return string(payload), nil
}

// Scan populates the JSONStringSlice from a database value.
func (s *JSONStringSlice) Scan(value any) error {
	if s == nil {
		return errors.New("json string slice scan: nil receiver")
	}
	if value == nil {
		*s = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.Errorf("json string slice scan: unsupported type %T", value)
	}

	if len(data) == 0 {
		*s = nil
		return nil
	}

	var decoded []string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return errors.Wrap(err, "unmarshal json string slice")
	}
	if len(decoded) == 0 {
		*s = nil
		return nil
	}
	*s = JSONStringSlice(decoded)
	return nil
}

// JSONStringMap stores a string map as JSON in the database.
type JSONStringMap map[string]string

// Value converts the JSONStringMap into a driver value.
func (m JSONStringMap) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(map[string]string(m))
	if err != nil {
		return nil, errors.Wrap(err, "marshal json string map")
	}
	return string(payload), nil
}

// Scan populates the JSONStringMap from a database value.
func (m *JSONStringMap) Scan(value any) error {
	if m == nil {
		return errors.New("json string map scan: nil receiver")
	}
	if value == nil {
		*m = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.Errorf("json string map scan: unsupported type %T", value)
	}

	if len(data) == 0 {
		*m = nil
		return nil
	}

	decoded := make(map[string]string)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return errors.Wrap(err, "unmarshal json string map")
	}
	if len(decoded) == 0 {
		*m = nil
		return nil
	}
	*m = JSONStringMap(decoded)
	return nil
}

// MCPToolPricingMap stores per-tool pricing as JSON in the database.
type MCPToolPricingMap map[string]ToolPricingLocal

// Value converts the MCPToolPricingMap into a driver value.
func (m MCPToolPricingMap) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	payload, err := json.Marshal(map[string]ToolPricingLocal(m))
	if err != nil {
		return nil, errors.Wrap(err, "marshal mcp tool pricing map")
	}
	return string(payload), nil
}

// Scan populates the MCPToolPricingMap from a database value.
func (m *MCPToolPricingMap) Scan(value any) error {
	if m == nil {
		return errors.New("mcp tool pricing map scan: nil receiver")
	}
	if value == nil {
		*m = nil
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.Errorf("mcp tool pricing map scan: unsupported type %T", value)
	}

	if len(data) == 0 {
		*m = nil
		return nil
	}

	decoded := make(map[string]ToolPricingLocal)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return errors.Wrap(err, "unmarshal mcp tool pricing map")
	}
	if len(decoded) == 0 {
		*m = nil
		return nil
	}
	*m = MCPToolPricingMap(decoded)
	return nil
}

// ToolPricingLocalJSON stores a tool pricing struct as JSON in the database.
type ToolPricingLocalJSON ToolPricingLocal

// Value converts the ToolPricingLocalJSON into a driver value.
func (p ToolPricingLocalJSON) Value() (driver.Value, error) {
	payload, err := json.Marshal(ToolPricingLocal(p))
	if err != nil {
		return nil, errors.Wrap(err, "marshal tool pricing")
	}
	if string(payload) == "null" {
		return nil, nil
	}
	return string(payload), nil
}

// Scan populates the ToolPricingLocalJSON from a database value.
func (p *ToolPricingLocalJSON) Scan(value any) error {
	if p == nil {
		return errors.New("tool pricing scan: nil receiver")
	}
	if value == nil {
		*p = ToolPricingLocalJSON{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return errors.Errorf("tool pricing scan: unsupported type %T", value)
	}

	if len(data) == 0 {
		*p = ToolPricingLocalJSON{}
		return nil
	}

	var decoded ToolPricingLocal
	if err := json.Unmarshal(data, &decoded); err != nil {
		return errors.Wrap(err, "unmarshal tool pricing")
	}
	*p = ToolPricingLocalJSON(decoded)
	return nil
}
