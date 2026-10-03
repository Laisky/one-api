package format

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResponseStateFormatDetectionBehaviorSelectorsAreRecognized verifies that
// stateful Responses requests without an input field are identified by automatic
// format detection, including requests that use a prompt template. Closes B11.
func TestResponseStateFormatDetectionBehaviorSelectorsAreRecognized(t *testing.T) {
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
			name: "conversation id only",
			body: `{"model":"gpt-5","conversation":"conv_123"}`,
		},
		{
			name: "conversation object only",
			body: `{"model":"gpt-5","conversation":{"id":"conv_123"}}`,
		},
		{
			name: "prompt template only",
			body: `{"model":"gpt-5","prompt":{"id":"pmpt_123"}}`,
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
			require.Equal(t, ResponseAPI, format)
		})
	}
}

// TestResponseStateFormatDetectionBehaviorStringPromptIsNotResponseAPI verifies
// that a legacy Completions-style string prompt is NOT misclassified as a
// Responses request; only the Responses prompt-template object triggers
// detection.
func TestResponseStateFormatDetectionBehaviorStringPromptIsNotResponseAPI(t *testing.T) {
	t.Parallel()

	format, err := DetectFormat([]byte(`{"model":"gpt-5","prompt":"say hello"}`))
	require.NoError(t, err)
	require.Equal(t, Unknown, format)
}

// TestResponseStateFormatDetectionBehaviorExplicitInputIsResponseAPI verifies
// that a state selector alongside the explicit input discriminator is still
// recognized as Responses.
func TestResponseStateFormatDetectionBehaviorExplicitInputIsResponseAPI(t *testing.T) {
	t.Parallel()

	format, err := DetectFormat([]byte(`{
		"model":"gpt-5",
		"conversation":"conv_123",
		"input":"continue"
	}`))
	require.NoError(t, err)
	require.Equal(t, ResponseAPI, format)
}
