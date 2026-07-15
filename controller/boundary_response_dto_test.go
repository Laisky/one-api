package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestChannelListItemMatchesSplicer_T9 proves the channelListItem replacement
// (embed dto.ChannelResponse + TestModels) serializes byte-identically to the
// retired byte-splicing MarshalJSON: the channel fields come first in the same
// order, followed by test_models, which is always an array (never null, never
// omitted). channelTextTestModels always returns a non-nil slice, so the
// reachable cases are 0/1/n models — exercised here (T9).
func TestChannelListItemMatchesSplicer_T9(t *testing.T) {
	weight := uint(3)
	base := (&model.Channel{
		Id:     91,
		UUID:   "018f0000-0000-7000-8000-00000000c001",
		Type:   1,
		Name:   "wrapper-channel",
		Status: 1,
		Weight: &weight,
		Models: "gpt-a,gpt-b",
		Group:  "default",
	}).ToResponse()

	baseJSON, err := json.Marshal(base)
	require.NoError(t, err)

	cases := map[string][]string{
		"zero": {},
		"one":  {"gpt-a"},
		"many": {"gpt-a", "gpt-b", "gpt-c"},
	}
	for name, testModels := range cases {
		t.Run(name, func(t *testing.T) {
			item := channelListItem{ChannelResponse: base, TestModels: testModels}
			got, err := json.Marshal(item)
			require.NoError(t, err)

			// Reconstruct exactly what the old byte-splicer emitted: channel
			// inner fields, then a comma, then "test_models":<array>.
			tmJSON, err := json.Marshal(testModels)
			require.NoError(t, err)
			inner := string(baseJSON[1 : len(baseJSON)-1])
			want := "{" + inner + `,"test_models":` + string(tmJSON) + "}"

			require.Equal(t, want, string(got), "channelListItem must match the retired byte-splicer output")

			// Structural guarantees: internal id absent, test_models is an array.
			var m map[string]any
			require.NoError(t, json.Unmarshal(got, &m))
			require.NotContains(t, m, "id", "wrapper must not leak the internal integer id")
			require.Contains(t, m, "uuid")
			arr, ok := m["test_models"].([]any)
			require.True(t, ok, "test_models must always be a JSON array, got %T", m["test_models"])
			require.Len(t, arr, len(testModels))
		})
	}
}
