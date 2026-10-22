package builtinname

import "testing"

// TestBuiltinReceiptNameIdentity keeps producer and billing normalization aligned.
func TestBuiltinReceiptNameIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw, known, canonical string
	}{
		{"web_search", "web_search", "web_search"},
		{" WEB_SEARCH_PREVIEW ", "web_search", "web_search"},
		{"web-search", "web_search", "web_search"},
		{"web_search_20250305", "web_search", "web_search"},
		{"web_fetch_20250910", "web_fetch", "web_fetch"},
		{"web_fetch", "", "web_fetch"},
		{"code_execution_20250825", "code_execution", "code_execution"},
		{"tool_search_tool_regex_20260209", "tool_search", "tool_search"},
		{"web_search_preview_reasoning", "", "web_search_preview_reasoning"},
		{"file_search", "", "file_search"},
		{"", "", ""},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			if got := Normalize(tc.raw); got != tc.known {
				t.Fatalf("Normalize(%q) = %q; want %q", tc.raw, got, tc.known)
			}
			if got := Canonical(tc.raw); got != tc.canonical {
				t.Fatalf("Canonical(%q) = %q; want %q", tc.raw, got, tc.canonical)
			}
		})
	}
}
