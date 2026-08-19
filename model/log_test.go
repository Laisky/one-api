package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLogMetadata_MarshalJSON_NilEmitsEmptyContainer(t *testing.T) {
	var m LogMetadata
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal nil LogMetadata: %v", err)
	}
	if string(got) != "{}" {
		t.Fatalf("nil LogMetadata => %s, want {}", got)
	}
}

func TestLogMetadata_MarshalJSON_NonNilEmitsValues(t *testing.T) {
	m := LogMetadata{"k": "v"}
	got, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal LogMetadata: %v", err)
	}
	if string(got) != `{"k":"v"}` {
		t.Fatalf("LogMetadata => %s, want {\"k\":\"v\"}", got)
	}
}

func TestLogMetadata_MarshalJSON_RoundTrip(t *testing.T) {
	original := LogMetadata{"k1": "v1", "k2": float64(42)}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded LogMetadata
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(map[string]any(original), map[string]any(decoded)) {
		t.Fatalf("round trip mismatch: got %v, want %v", decoded, original)
	}
}

// Confirms the metadata field on Log uses ",omitempty" — the tag on the field
// is `json:"metadata,omitempty"`, so a nil map should be omitted rather than
// emitted as null or {}.
func TestLogJSON_NilMetadataOmitsOrEmits(t *testing.T) {
	l := Log{}
	raw, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("marshal Log: %v", err)
	}
	got := string(raw)
	if strings.Contains(got, `"metadata"`) {
		t.Fatalf("expected metadata omitted (omitempty tag) but got %s", got)
	}
	if strings.Contains(got, `"metadata":null`) {
		t.Fatalf("found metadata:null in %s", got)
	}
}

func TestLogMetadata_GormScanNullThenMarshalEmitsEmptyObject(t *testing.T) {
	var m LogMetadata
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

func TestLogMetadata_GormScanEmptyByteSliceThenMarshalEmitsEmptyObject(t *testing.T) {
	var m LogMetadata
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

func TestLogMetadata_GormScanEmptyStringThenMarshalEmitsEmptyObject(t *testing.T) {
	var m LogMetadata
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
