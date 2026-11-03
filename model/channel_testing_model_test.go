package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeTestingModel pins the shared testing_model storage policy: nil for
// auto-select, the canonical SKIP sentinel for any SKIP spelling, or a trimmed model the
// channel serves by the same SupportsModel rule the health checker applies.
// Parameters: t is the test handle. Returns: no values.
func TestNormalizeTestingModel(t *testing.T) {
	t.Parallel()
	skip := ChannelTestingModelSkip
	listed := &Channel{Models: " gpt-4o-mini , gpt-4o", ModelMapping: resetTestString(`{"alias":"gpt-4o","dangling":"gpt-5"}`)}
	unrestricted := &Channel{}
	cases := []struct {
		name    string
		channel *Channel
		value   *string
		want    *string
	}{
		{name: "nil selects auto", channel: listed, value: nil, want: nil},
		{name: "blank selects auto", channel: listed, value: resetTestString("   "), want: nil},
		{name: "canonical SKIP", channel: listed, value: resetTestString(skip), want: &skip},
		{name: "upper-case SKIP", channel: listed, value: resetTestString("__SKIP__"), want: &skip},
		{name: "padded mixed-case SKIP", channel: listed, value: resetTestString("  __Skip__\t"), want: &skip},
		{name: "listed model", channel: listed, value: resetTestString("gpt-4o"), want: resetTestString("gpt-4o")},
		{name: "listed model is trimmed", channel: listed, value: resetTestString(" gpt-4o-mini "), want: resetTestString("gpt-4o-mini")},
		{name: "mapping alias of a listed model", channel: listed, value: resetTestString("alias"), want: resetTestString("alias")},
		{name: "mapping alias of an unlisted model", channel: listed, value: resetTestString("dangling"), want: nil},
		{name: "unlisted model selects auto", channel: listed, value: resetTestString("gpt-5"), want: nil},
		{name: "model names are case-sensitive", channel: listed, value: resetTestString("GPT-4O"), want: nil},
		{name: "near-miss sentinel is a model name", channel: listed, value: resetTestString("__skip"), want: nil},
		{name: "unrestricted channel keeps a named model", channel: unrestricted, value: resetTestString(" gpt-4o "), want: resetTestString("gpt-4o")},
		{name: "unrestricted channel keeps SKIP", channel: unrestricted, value: resetTestString(skip), want: &skip},
		{name: "unrestricted channel blank selects auto", channel: unrestricted, value: resetTestString(""), want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.channel.NormalizeTestingModel(tc.value)
			require.Equal(t, tc.want, got)
			if got != nil && tc.value != nil {
				require.NotSame(t, tc.value, got, "the result must not alias the caller's pointer")
			}
		})
	}
}

// TestResolveTestingModelUpdate pins the merge-patch update semantics: an omitted field
// keeps the latest stored value, an explicit null or blank clears it, and the result is
// always normalized against the channel as it stands after the update.
// Parameters: t is the test handle. Returns: no values.
func TestResolveTestingModelUpdate(t *testing.T) {
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
			current := &Channel{Models: tc.models, TestingModel: tc.stored}
			require.Equal(t, tc.want, resolveTestingModelUpdate(current, tc.requested, tc.provided))
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

// TestClearStaleChannelTestingModel pins the compare-and-swap cleanup the health
// checker uses: only the value it observed is cleared, so a selection saved after the
// probe read the channel survives.
// Parameters: t is the test handle. Returns: no values.
func TestClearStaleChannelTestingModel(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	ctx := context.Background()
	skip := ChannelTestingModelSkip
	stale := "sora-2"
	channel := &Channel{Name: "stale-cleanup", Models: "sora-2,gpt-4o-mini", Group: "default",
		Status: ChannelStatusEnabled, TestingModel: &stale}
	require.NoError(t, channel.Insert())
	load := func() *string {
		var stored Channel
		require.NoError(t, db.Select("id", "testing_model").First(&stored, "id = ?", channel.Id).Error)
		return stored.TestingModel
	}

	cleared, err := ClearStaleChannelTestingModel(ctx, channel.Id, nil)
	require.NoError(t, err)
	require.False(t, cleared, "nothing observed means nothing to clear")
	require.Equal(t, &stale, load())

	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("testing_model", skip).Error)
	cleared, err = ClearStaleChannelTestingModel(ctx, channel.Id, &stale)
	require.NoError(t, err)
	require.False(t, cleared, "a selection replaced after the probe read it must be kept")
	require.Equal(t, &skip, load())

	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("testing_model", stale).Error)
	cleared, err = ClearStaleChannelTestingModel(ctx, channel.Id, &stale)
	require.NoError(t, err)
	require.True(t, cleared)
	require.Nil(t, load())
}

// TestRefreshTestingModelWithContext verifies a sweep can replace its snapshot of a
// channel's testing model with the stored value, and that a failed read leaves the
// snapshot untouched.
// Parameters: t is the test handle. Returns: no values.
func TestRefreshTestingModelWithContext(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	ctx := context.Background()
	channel := &Channel{Name: "refresh", Models: "gpt-4o", Group: "default", Status: ChannelStatusEnabled}
	require.NoError(t, channel.Insert())
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("testing_model", ChannelTestingModelSkip).Error)

	snapshot := *channel
	require.Nil(t, snapshot.TestingModel)
	require.NoError(t, snapshot.RefreshTestingModelWithContext(ctx))
	require.True(t, snapshot.IsTestingSkipped())

	missing := &Channel{Id: channel.Id + 1000, TestingModel: resetTestString("gpt-4o")}
	require.Error(t, missing.RefreshTestingModelWithContext(ctx))
	require.Equal(t, resetTestString("gpt-4o"), missing.TestingModel)
}
