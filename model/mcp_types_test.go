package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestJSONStringSlice_MarshalJSON_NilEmitsEmptyContainer(t *testing.T) {
	var s JSONStringSlice
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal nil JSONStringSlice: %v", err)
	}
	if string(got) != "[]" {
		t.Fatalf("nil JSONStringSlice => %s, want []", got)
	}
}

func TestJSONStringSlice_MarshalJSON_NonNilEmitsValues(t *testing.T) {
	s := JSONStringSlice{"a", "b"}
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal JSONStringSlice: %v", err)
	}
	if string(got) != `["a","b"]` {
		t.Fatalf("JSONStringSlice => %s, want [\"a\",\"b\"]", got)
	}
}

func TestJSONStringSlice_MarshalJSON_RoundTrip(t *testing.T) {
	original := JSONStringSlice{"x", "y", "z"}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded JSONStringSlice
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual([]string(original), []string(decoded)) {
		t.Fatalf("round trip mismatch: got %v, want %v", decoded, original)
	}
}

func TestJSONStringMap_MarshalJSON_NilEmitsEmptyContainer(t *testing.T) {
	var m JSONStringMap
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal nil JSONStringMap: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("nil JSONStringMap => %s, want {}", got)
	}
}

func TestJSONStringMap_MarshalJSON_NonNilEmitsValues(t *testing.T) {
	m := JSONStringMap{"k": "v"}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal JSONStringMap: %v", err)
	}
	if string(got) != `{"k":"v"}` {
		t.Fatalf("JSONStringMap => %s, want {\"k\":\"v\"}", got)
	}
}

func TestJSONStringMap_MarshalJSON_RoundTrip(t *testing.T) {
	original := JSONStringMap{"k1": "v1", "k2": "v2"}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded JSONStringMap
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(map[string]string(original), map[string]string(decoded)) {
		t.Fatalf("round trip mismatch: got %v, want %v", decoded, original)
	}
}

func TestMCPToolPricingMap_MarshalJSON_NilEmitsEmptyContainer(t *testing.T) {
	var m MCPToolPricingMap
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal nil MCPToolPricingMap: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("nil MCPToolPricingMap => %s, want {}", got)
	}
}

func TestMCPToolPricingMap_MarshalJSON_NonNilEmitsValues(t *testing.T) {
	m := MCPToolPricingMap{"tool_a": ToolPricingLocal{}}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal MCPToolPricingMap: %v", err)
	}
	if !strings.Contains(string(got), `"tool_a"`) {
		t.Fatalf("MCPToolPricingMap => %s, missing tool_a key", got)
	}
}

func TestMCPToolPricingMap_MarshalJSON_RoundTrip(t *testing.T) {
	original := MCPToolPricingMap{"tool_a": ToolPricingLocal{}}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded MCPToolPricingMap
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded) != len(original) {
		t.Fatalf("round trip length mismatch: got %d, want %d", len(decoded), len(original))
	}
	if _, ok := decoded["tool_a"]; !ok {
		t.Fatalf("round trip missing tool_a key")
	}
}

func TestUserJSON_NilMCPToolBlacklistEmitsEmptyArray(t *testing.T) {
	u := User{}
	raw, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal User: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, `"mcp_tool_blacklist":[]`) {
		t.Fatalf("expected mcp_tool_blacklist:[] in %s", got)
	}
	if strings.Contains(got, `"mcp_tool_blacklist":null`) {
		t.Fatalf("found mcp_tool_blacklist:null in %s", got)
	}
}

func TestMCPServerJSON_NilFieldsEmitEmptyContainers(t *testing.T) {
	s := MCPServer{}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal MCPServer: %v", err)
	}
	got := string(raw)

	expectations := []string{
		`"headers":{}`,
		`"tool_whitelist":[]`,
		`"tool_blacklist":[]`,
		`"tool_pricing":{}`,
	}
	for _, want := range expectations {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %s in %s", want, got)
		}
	}

	forbidden := []string{
		`"headers":null`,
		`"tool_whitelist":null`,
		`"tool_blacklist":null`,
		`"tool_pricing":null`,
	}
	for _, bad := range forbidden {
		if strings.Contains(got, bad) {
			t.Fatalf("found forbidden %s in %s", bad, got)
		}
	}
}

func TestJSONStringSlice_GormScanNullThenMarshalEmitsEmptyArray(t *testing.T) {
	var s JSONStringSlice
	if err := s.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "[]" {
		t.Fatalf("after Scan(nil): %s, want []", got)
	}
}

func TestJSONStringSlice_GormScanEmptyByteSliceThenMarshalEmitsEmptyArray(t *testing.T) {
	var s JSONStringSlice
	if err := s.Scan([]byte("")); err != nil {
		t.Fatalf("Scan([]byte{}): %v", err)
	}
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "[]" {
		t.Fatalf("after Scan([]byte{}): %s, want []", got)
	}
}

func TestJSONStringSlice_GormScanEmptyStringThenMarshalEmitsEmptyArray(t *testing.T) {
	var s JSONStringSlice
	if err := s.Scan(""); err != nil {
		t.Fatalf("Scan(\"\"): %v", err)
	}
	got, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "[]" {
		t.Fatalf("after Scan(\"\"): %s, want []", got)
	}
}

func TestJSONStringMap_GormScanNullThenMarshalEmitsEmptyObject(t *testing.T) {
	var m JSONStringMap
	if err := m.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("after Scan(nil): %s, want {}", got)
	}
}

func TestJSONStringMap_GormScanEmptyByteSliceThenMarshalEmitsEmptyObject(t *testing.T) {
	var m JSONStringMap
	if err := m.Scan([]byte("")); err != nil {
		t.Fatalf("Scan([]byte{}): %v", err)
	}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("after Scan([]byte{}): %s, want {}", got)
	}
}

func TestJSONStringMap_GormScanEmptyStringThenMarshalEmitsEmptyObject(t *testing.T) {
	var m JSONStringMap
	if err := m.Scan(""); err != nil {
		t.Fatalf("Scan(\"\"): %v", err)
	}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("after Scan(\"\"): %s, want {}", got)
	}
}

func TestMCPToolPricingMap_GormScanNullThenMarshalEmitsEmptyObject(t *testing.T) {
	var m MCPToolPricingMap
	if err := m.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("after Scan(nil): %s, want {}", got)
	}
}

func TestMCPToolPricingMap_GormScanEmptyByteSliceThenMarshalEmitsEmptyObject(t *testing.T) {
	var m MCPToolPricingMap
	if err := m.Scan([]byte("")); err != nil {
		t.Fatalf("Scan([]byte{}): %v", err)
	}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("after Scan([]byte{}): %s, want {}", got)
	}
}

func TestMCPToolPricingMap_GormScanEmptyStringThenMarshalEmitsEmptyObject(t *testing.T) {
	var m MCPToolPricingMap
	if err := m.Scan(""); err != nil {
		t.Fatalf("Scan(\"\"): %v", err)
	}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("after Scan(\"\"): %s, want {}", got)
	}
}
