package gemini

import "testing"

// TestNativeThinkingLevelContract exercises defaults and invalid values without
// requiring credentials, network access, or global catalog mutation.
func TestNativeThinkingLevelContract(t *testing.T) {
	for _, tc := range []struct {
		name, requested, defaultLevel, want string
		nilInput, wantError                 bool
	}{
		{name: "absent-lite", nilInput: true, defaultLevel: "minimal", want: "minimal"},
		{name: "empty-lite", defaultLevel: "minimal", want: "minimal"},
		{name: "default-lite", requested: "default", defaultLevel: "minimal", want: "minimal"},
		{name: "minimal", requested: "minimal", want: "minimal"},
		{name: "high", requested: "high", want: "high"},
		{name: "normalized", requested: " HIGH ", want: "high"},
		{name: "low-rejected", requested: "low", wantError: true},
		{name: "medium-rejected", requested: "medium", wantError: true},
		{name: "none-rejected", requested: "none", wantError: true},
		{name: "unknown-rejected", requested: "arbitrary", wantError: true},
		{name: "legacy-default-omitted", nilInput: true},
		{name: "bad-contract-default", defaultLevel: "medium", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requested := &tc.requested
			if tc.nilInput {
				requested = nil
			}
			got, err := resolveNativeThinkingLevel(requested, []string{"minimal", "high"}, tc.defaultLevel)
			if (err != nil) != tc.wantError || got != tc.want {
				t.Fatalf("level = %q, error = %v; want %q, error %v", got, err, tc.want, tc.wantError)
			}
		})
	}
}
