package deepseekcompat

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func intPtr(v int) *int {
	return &v
}

func TestNormalizeThinkingType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		rawType          string
		budgetTokens     *int
		expectedNorm     string
		expectedChanged  bool
	}{
		{name: "enabled unchanged", rawType: "enabled", budgetTokens: intPtr(1024), expectedNorm: "enabled", expectedChanged: false},
		{name: "disabled unchanged", rawType: "disabled", budgetTokens: nil, expectedNorm: "disabled", expectedChanged: false},
		{name: "adaptive to enabled", rawType: "adaptive", budgetTokens: nil, expectedNorm: "enabled", expectedChanged: true},
		{name: "adaptive with budget to enabled", rawType: "adaptive", budgetTokens: intPtr(4096), expectedNorm: "enabled", expectedChanged: true},
		{name: "empty with budget to enabled", rawType: "", budgetTokens: intPtr(2048), expectedNorm: "enabled", expectedChanged: true},
		{name: "empty without budget to disabled", rawType: "", budgetTokens: nil, expectedNorm: "disabled", expectedChanged: true},
		{name: "unknown with budget to enabled", rawType: "auto", budgetTokens: intPtr(1024), expectedNorm: "enabled", expectedChanged: true},
		{name: "unknown without budget to disabled", rawType: "auto", budgetTokens: nil, expectedNorm: "disabled", expectedChanged: true},
		{name: "ENABLED case normalized", rawType: "ENABLED", budgetTokens: nil, expectedNorm: "enabled", expectedChanged: true},
		{name: "enabled nil budget unchanged", rawType: "enabled", budgetTokens: nil, expectedNorm: "enabled", expectedChanged: false},
		{name: "zero budget treated as no budget", rawType: "", budgetTokens: intPtr(0), expectedNorm: "disabled", expectedChanged: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			normalized, changed := NormalizeThinkingType(tc.rawType, tc.budgetTokens)
			assert.Equal(t, tc.expectedNorm, normalized)
			assert.Equal(t, tc.expectedChanged, changed)
		})
	}
}
