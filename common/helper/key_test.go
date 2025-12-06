package helper

import "testing"

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "normal key (long)",
			key:      "sk-1234567890abcdefghij",
			expected: "sk-123...ghij",
		},
		{
			name:     "exactly 12 chars",
			key:      "123456789012",
			expected: "123456...9012",
		},
		{
			name:     "short key (less than 12)",
			key:      "short",
			expected: "***",
		},
		{
			name:     "empty key",
			key:      "",
			expected: "***",
		},
		{
			name:     "11 chars (just under threshold)",
			key:      "12345678901",
			expected: "***",
		},
		{
			name:     "real world API key format",
			key:      "sk-proj-abc123def456ghi789jkl012mno345",
			expected: "sk-pro...o345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskAPIKey(tt.key)
			if result != tt.expected {
				t.Errorf("MaskAPIKey(%q) = %q, expected %q", tt.key, result, tt.expected)
			}
		})
	}
}
