package aws

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Mirrors relay/adaptor/aws/mistral/model_id_internal_test.go: the public test
// file's TestAwsModelID / TestConvertStopReason bodies are
// `_ = test // Placeholder to avoid unused variable error`, because an external
// test package cannot reach the unexported functions. This internal file can.

// TestAwsModelIDMapsEveryAdvertisedModel pins the request-model to Bedrock-model-id
// mapping. A wrong or missing entry makes the model uninvokable on AWS.
func TestAwsModelIDMapsEveryAdvertisedModel(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, AwsModelIDMap)
	for requestModel, wantBedrockID := range AwsModelIDMap {
		t.Run(requestModel, func(t *testing.T) {
			t.Parallel()
			got, err := awsModelID(requestModel)
			require.NoError(t, err)
			require.Equal(t, wantBedrockID, got)
		})
	}
}

// TestAwsModelIDRejectsUnknownModel pins that an unmapped model is an error rather
// than an empty id silently sent to Bedrock.
func TestAwsModelIDRejectsUnknownModel(t *testing.T) {
	t.Parallel()

	got, err := awsModelID("definitely-not-a-deepseek-model")
	require.Error(t, err)
	require.Empty(t, got)
}

// TestConvertStopReasonNormalizesUpstreamValues pins the stop-reason mapping.
// DeepSeek on Bedrock emits Anthropic-style end_turn/max_tokens alongside the
// OpenAI-style values, and both must normalize.
func TestConvertStopReasonNormalizesUpstreamValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want *string
	}{
		{in: "stop", want: ptr("stop")},
		{in: "end_turn", want: ptr("stop")},
		{in: "length", want: ptr("length")},
		{in: "max_tokens", want: ptr("length")},
		{in: "something_new", want: ptr("stop")},
		{in: "", want: nil},
	} {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := convertStopReason(tc.in)
			if tc.want == nil {
				require.Nil(t, got, "an empty upstream reason must stay absent")
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tc.want, *got)
		})
	}
}

// ptr returns a pointer to v.
//
// Parameters:
//   - v: the value to address.
//
// Return values:
//   - *string: a pointer to a copy of v.
func ptr(v string) *string { return &v }
