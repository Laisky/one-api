package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeChannelTestingModel pins the shared testing_model storage policy: nil
// for auto-select, the canonical SKIP sentinel for any SKIP spelling, or a trimmed
// model the channel lists.
// Parameters: t is the test handle. Returns: no values.
func TestNormalizeChannelTestingModel(t *testing.T) {
	t.Parallel()
	const models = " gpt-4o-mini , gpt-4o"
	skip := ChannelTestingModelSkip
	cases := []struct {
		name  string
		value *string
		want  *string
	}{
		{name: "nil selects auto", value: nil, want: nil},
		{name: "blank selects auto", value: resetTestString("   "), want: nil},
		{name: "canonical SKIP", value: resetTestString(skip), want: &skip},
		{name: "upper-case SKIP", value: resetTestString("__SKIP__"), want: &skip},
		{name: "padded mixed-case SKIP", value: resetTestString("  __Skip__\t"), want: &skip},
		{name: "listed model", value: resetTestString("gpt-4o"), want: resetTestString("gpt-4o")},
		{name: "listed model is trimmed", value: resetTestString(" gpt-4o-mini "), want: resetTestString("gpt-4o-mini")},
		{name: "unlisted model selects auto", value: resetTestString("gpt-5"), want: nil},
		{name: "model names are case-sensitive", value: resetTestString("GPT-4O"), want: nil},
		{name: "near-miss sentinel is a model name", value: resetTestString("__skip"), want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeChannelTestingModel(tc.value, models)
			require.Equal(t, tc.want, got)
			if got != nil && tc.value != nil {
				require.NotSame(t, tc.value, got, "the result must not alias the caller's pointer")
			}
		})
	}

	t.Run("SKIP needs no model list", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, &skip, NormalizeChannelTestingModel(resetTestString(skip), ""))
		require.Nil(t, NormalizeChannelTestingModel(resetTestString("gpt-4o"), ""))
	})
}

// TestResolveUpdatedTestingModel pins the merge-patch update semantics: an omitted
// field keeps the stored value, an explicit null or blank clears it, and the result is
// always normalized against the effective model list.
// Parameters: t is the test handle. Returns: no values.
func TestResolveUpdatedTestingModel(t *testing.T) {
	t.Parallel()
	const models = "gpt-4o-mini,gpt-4o"
	skip := ChannelTestingModelSkip
	cases := []struct {
		name      string
		requested *string
		provided  bool
		stored    *string
		models    string
		want      *string
	}{
		{name: "omitted keeps SKIP", stored: &skip, models: models, want: &skip},
		{name: "omitted keeps SKIP across a model change", stored: &skip, models: "other", want: &skip},
		{name: "omitted keeps a listed model", stored: resetTestString("gpt-4o"), models: models, want: resetTestString("gpt-4o")},
		{name: "omitted drops a model no longer listed", stored: resetTestString("gpt-4o"), models: "gpt-4o-mini", want: nil},
		{name: "omitted canonicalises a legacy SKIP", stored: resetTestString("__SKIP__"), models: models, want: &skip},
		{name: "omitted keeps auto", models: models, want: nil},
		{name: "null clears a listed model", provided: true, stored: resetTestString("gpt-4o"), models: models, want: nil},
		{name: "null clears SKIP", provided: true, stored: &skip, models: models, want: nil},
		{name: "blank clears", requested: resetTestString(""), provided: true, stored: resetTestString("gpt-4o"), models: models, want: nil},
		{name: "SKIP replaces a model", requested: &skip, provided: true, stored: resetTestString("gpt-4o"), models: models, want: &skip},
		{name: "listed model replaces SKIP", requested: resetTestString("gpt-4o-mini"), provided: true, stored: &skip, models: models, want: resetTestString("gpt-4o-mini")},
		{name: "unlisted request selects auto", requested: resetTestString("gpt-5"), provided: true, stored: &skip, models: models, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, resolveUpdatedTestingModel(tc.requested, tc.provided, tc.stored, tc.models))
		})
	}
}

// TestChannelTestingSkipRecognition verifies the model-layer SKIP predicate the health
// checker delegates to, and the read-side canonicalisation used by API responses.
// Parameters: t is the test handle. Returns: no values.
func TestChannelTestingSkipRecognition(t *testing.T) {
	t.Parallel()
	for _, spelling := range []string{ChannelTestingModelSkip, "__SKIP__", " __Skip__ "} {
		require.True(t, IsChannelTestingModelSkip(spelling), spelling)
		require.True(t, (&Channel{TestingModel: resetTestString(spelling)}).IsTestingSkipped(), spelling)
		require.Equal(t, ChannelTestingModelSkip, *canonicalChannelTestingModel(resetTestString(spelling)), spelling)
	}
	for _, other := range []string{"", "skip", "gpt-4o", "__skip"} {
		require.False(t, IsChannelTestingModelSkip(other), other)
		value := resetTestString(other)
		require.Same(t, value, canonicalChannelTestingModel(value), "non-SKIP values are reported unchanged")
	}

	var nilChannel *Channel
	require.False(t, nilChannel.IsTestingSkipped())
	require.False(t, (&Channel{}).IsTestingSkipped())
	require.Nil(t, canonicalChannelTestingModel(nil))

	legacy := " __SKIP__ "
	response := (&Channel{TestingModel: &legacy}).ToResponse()
	require.Equal(t, ChannelTestingModelSkip, *response.TestingModel)
	require.Equal(t, " __SKIP__ ", legacy, "rendering a response must not rewrite the channel")
}

// TestChannelPersistenceKeepsTestingModelSkip exercises the model persistence entry
// points directly, including internal Update callers that never populate
// NullableFieldsProvided, so SKIP cannot be lost below the HTTP layer.
// Parameters: t is the test handle. Returns: no values.
func TestChannelPersistenceKeepsTestingModelSkip(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	skip := ChannelTestingModelSkip
	loadTestingModel := func(id int) *string {
		var stored Channel
		require.NoError(t, db.Select("id", "testing_model").First(&stored, "id = ?", id).Error)
		return stored.TestingModel
	}

	inserted := &Channel{Name: "insert-skip", Models: "Foo", Group: "default", Status: ChannelStatusEnabled,
		TestingModel: resetTestString("__SKIP__")}
	require.NoError(t, inserted.Insert())
	require.Equal(t, &skip, loadTestingModel(inserted.Id), "Insert stores the canonical sentinel")

	batch := []Channel{
		{Name: "batch-skip", Models: "Foo", Group: "default", Status: ChannelStatusEnabled, TestingModel: resetTestString(skip)},
		{Name: "batch-unlisted", Models: "Foo", Group: "default", Status: ChannelStatusEnabled, TestingModel: resetTestString("Bar")},
	}
	require.NoError(t, BatchInsertChannels(batch))
	require.Equal(t, &skip, loadTestingModel(batch[0].Id))
	require.Nil(t, loadTestingModel(batch[1].Id), "an unlisted model is not persisted at creation")

	// An internal caller that reloads the full row and saves it back, as the pricing
	// endpoint does, must keep SKIP.
	reloaded, err := GetChannelById(inserted.Id, false)
	require.NoError(t, err)
	require.NoError(t, reloaded.UpdateWithContext(context.Background()))
	require.Equal(t, &skip, loadTestingModel(inserted.Id))

	// A partial update that omits testing_model and changes the model list keeps SKIP.
	partial := &Channel{Id: inserted.Id, Name: "insert-skip", Models: "Bar"}
	require.NoError(t, partial.Update())
	require.Equal(t, &skip, loadTestingModel(inserted.Id))
	require.Equal(t, &skip, partial.TestingModel, "the reloaded channel reflects the persisted value")

	// An explicit null, signalled through NullableFieldsProvided, returns to auto-select.
	clearRequest := &Channel{Id: inserted.Id, Name: "insert-skip",
		NullableFieldsProvided: map[string]bool{ChannelTestingModelField: true}}
	require.NoError(t, clearRequest.Update())
	require.Nil(t, loadTestingModel(inserted.Id))
}
