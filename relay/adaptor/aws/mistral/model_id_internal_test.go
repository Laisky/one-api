package aws

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The public test file (package aws_test) carries a TestAwsModelID whose subtests
// are `_ = test // Placeholder to avoid unused variable error`, justified by
// "awsModelID is not exported, so we can't test it directly". That is true from an
// external test package — the fix is an internal one, which this file is. The
// placeholder's table was also wrong: it expected mistral-large-2407, an id
// AwsModelIDMap has never contained.

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

	got, err := awsModelID("definitely-not-a-mistral-model")
	require.Error(t, err)
	require.Empty(t, got)
}

// TestConvertStopReasonNormalizesUpstreamValues pins the stop-reason mapping,
// including that an absent reason stays absent rather than becoming "stop".
func TestConvertStopReasonNormalizesUpstreamValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   string
		want *string
	}{
		{in: "stop", want: ptr("stop")},
		{in: "length", want: ptr("length")},
		{in: "tool_calls", want: ptr("tool_calls")},
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
