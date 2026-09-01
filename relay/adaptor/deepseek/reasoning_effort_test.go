package deepseek

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestNormalizeDeepSeekReasoningEffortMatchesOfficialMapping verifies the
// current DeepSeek low, medium, high, xhigh, and max compatibility mapping.
// Parameters: t is the testing handle used for assertions and subtests.
// Returns: nothing; the test fails when normalization diverges from the official mapping.
func TestNormalizeDeepSeekReasoningEffortMatchesOfficialMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantNil bool
	}{
		{name: "low remains low", input: " low ", want: "low"},
		{name: "medium maps to high", input: "medium", want: "high"},
		{name: "high remains high", input: "HIGH", want: "high"},
		{name: "xhigh maps to high", input: "xhigh", want: "high"},
		{name: "max remains max", input: "max", want: "max"},
		{name: "unsupported value is cleared", input: "minimal", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			effort := tt.input
			request := &model.GeneralOpenAIRequest{ReasoningEffort: &effort}
			normalizeDeepSeekReasoningEffort(request)

			if tt.wantNil {
				require.Nil(t, request.ReasoningEffort)
				return
			}
			require.NotNil(t, request.ReasoningEffort)
			require.Equal(t, tt.want, *request.ReasoningEffort)
		})
	}
}

// TestNormalizeDeepSeekReasoningEffortAllowsNil verifies nil inputs remain a no-op.
// Parameters: t is the testing handle used for the panic assertion.
// Returns: nothing; the test fails if a nil input causes a panic.
func TestNormalizeDeepSeekReasoningEffortAllowsNil(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		normalizeDeepSeekReasoningEffort(nil)
		normalizeDeepSeekReasoningEffort(&model.GeneralOpenAIRequest{})
	})
}
