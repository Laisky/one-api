package format

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseStateFormatDetectionBehaviorSelectorsAreNotRecognized verifies
// that stateful Responses requests without an input field are not identified by
// automatic format detection, including requests that use a prompt template.
func TestResponseStateFormatDetectionBehaviorSelectorsAreNotRecognized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "previous response only",
			body: `{"model":"gpt-5","previous_response_id":"resp_123"}`,
		},
		{
			name: "conversation only",
			body: `{"model":"gpt-5","conversation":"conv_123"}`,
		},
		{
			name: "previous response with prompt template",
			body: `{"model":"gpt-5","previous_response_id":"resp_123","prompt":{"id":"pmpt_123"}}`,
		},
		{
			name: "conversation with prompt template",
			body: `{"model":"gpt-5","conversation":"conv_123","prompt":{"id":"pmpt_123"}}`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			format, err := DetectFormat([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, Unknown, format)
		})
	}
}

// TestResponseStateFormatDetectionBehaviorExplicitInputMasksSelectorGap
// verifies that a state selector is recognized only incidentally when the same
// payload also contains the existing input discriminator.
func TestResponseStateFormatDetectionBehaviorExplicitInputMasksSelectorGap(t *testing.T) {
	t.Parallel()

	format, err := DetectFormat([]byte(`{
		"model":"gpt-5",
		"conversation":"conv_123",
		"input":"continue"
	}`))
	require.NoError(t, err)
	require.Equal(t, ResponseAPI, format)
}
