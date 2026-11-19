package adaptor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSupportedReasoningEffort pins the shared catalog effort policy: only a
// listed value survives (normalized), and every other input is dropped. It
// accepts t and returns no values.
func TestSupportedReasoningEffort(t *testing.T) {
	t.Parallel()
	catalog := map[string]ModelConfig{
		"listed":     {SupportedReasoningEfforts: []string{"none", "high"}},
		"no-efforts": {SupportedFeatures: []string{"reasoning"}},
	}
	for _, tc := range []struct {
		name   string
		model  string
		effort *string
		want   *string
	}{
		{name: "listed value", model: "listed", effort: new("high"), want: new("high")},
		{name: "listed none", model: "listed", effort: new("none"), want: new("none")},
		{name: "case and whitespace variant", model: "listed", effort: new(" HIGH\t"), want: new("high")},
		{name: "globally valid but unlisted", model: "listed", effort: new("max"), want: nil},
		{name: "invalid value", model: "listed", effort: new("extreme"), want: nil},
		{name: "blank value", model: "listed", effort: new("  "), want: nil},
		{name: "nil effort", model: "listed", effort: nil, want: nil},
		{name: "model without vocabulary", model: "no-efforts", effort: new("high"), want: nil},
		{name: "unknown model", model: "unknown", effort: new("high"), want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var before string
			if tc.effort != nil {
				before = *tc.effort
			}
			got := SupportedReasoningEffort(catalog, tc.model, tc.effort)
			require.Equal(t, tc.want, got)
			if tc.effort != nil {
				require.Equal(t, before, *tc.effort, "caller value must not be mutated")
				if got != nil {
					require.NotSame(t, tc.effort, got, "result must not alias the caller value")
				}
			}
		})
	}
	require.Nil(t, SupportedReasoningEffort(nil, "listed", new("high")), "nil catalog drops the field")
}
