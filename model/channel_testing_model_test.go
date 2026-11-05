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

// TestClearStaleChannelTestingModel pins the revision-checked cleanup the health checker
// uses: a stale selection is cleared only while the channel still has the probe
// configuration the decision was based on, so a selection saved since, or a model list
// extended since, survives.
// Parameters: t is the test handle. Returns: no values.
func TestClearStaleChannelTestingModel(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	ctx := context.Background()
	stale := "sora-2"
	channel := &Channel{Name: "stale-cleanup", Models: "sora-2,gpt-4o-mini", Group: "default",
		Status: ChannelStatusEnabled, TestingModel: &stale}
	require.NoError(t, channel.Insert())
	snapshot := func() *Channel {
		probed := &Channel{Id: channel.Id}
		require.NoError(t, probed.ReloadForProbeWithContext(ctx))
		return probed
	}
	load := func() *string { return snapshot().TestingModel }

	t.Run("a replaced selection is kept", func(t *testing.T) {
		revision := snapshot().ProbeConfigRevision()
		require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("testing_model", ChannelTestingModelSkip).Error)
		cleared, err := ClearStaleChannelTestingModel(ctx, channel.Id, revision)
		require.NoError(t, err)
		require.False(t, cleared)
		require.Equal(t, resetTestString(ChannelTestingModelSkip), load())
		require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("testing_model", stale).Error)
	})

	t.Run("a changed model list keeps the selection", func(t *testing.T) {
		revision := snapshot().ProbeConfigRevision()
		require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("models", "sora-2,gpt-4o-mini,gpt-4o").Error)
		cleared, err := ClearStaleChannelTestingModel(ctx, channel.Id, revision)
		require.NoError(t, err)
		require.False(t, cleared, "the decision was made against a model list that no longer exists")
		require.Equal(t, &stale, load())
	})

	t.Run("an unchanged channel is repaired", func(t *testing.T) {
		cleared, err := ClearStaleChannelTestingModel(ctx, channel.Id, snapshot().ProbeConfigRevision())
		require.NoError(t, err)
		require.True(t, cleared)
		require.Nil(t, load())
	})

	t.Run("an empty selection has nothing to clear", func(t *testing.T) {
		cleared, err := ClearStaleChannelTestingModel(ctx, channel.Id, snapshot().ProbeConfigRevision())
		require.NoError(t, err)
		require.False(t, cleared)
	})
}

// TestReloadForProbeWithContext verifies a sweep replaces its whole snapshot of a channel
// with the stored row, so the model list, mapping, and selection it judges are mutually
// consistent, and that a failed read leaves the snapshot untouched.
// Parameters: t is the test handle. Returns: no values.
func TestReloadForProbeWithContext(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	ctx := context.Background()
	channel := &Channel{Name: "reload", Key: "sk-test", Models: "gpt-4o-mini", Group: "default", Status: ChannelStatusEnabled}
	require.NoError(t, channel.Insert())
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).
		Updates(map[string]any{"models": "gpt-4o", "testing_model": "gpt-4o", "status": ChannelStatusAutoDisabled}).Error)

	snapshot := *channel
	require.NoError(t, snapshot.ReloadForProbeWithContext(ctx))
	require.Equal(t, "gpt-4o", snapshot.Models)
	require.Equal(t, resetTestString("gpt-4o"), snapshot.TestingModel)
	require.Equal(t, ChannelStatusAutoDisabled, snapshot.Status)
	require.Equal(t, "sk-test", snapshot.Key, "the probe needs the key")
	require.NotEqual(t, channel.ProbeConfigRevision(), snapshot.ProbeConfigRevision())

	missing := &Channel{Id: channel.Id + 1000, Models: "kept"}
	require.Error(t, missing.ReloadForProbeWithContext(ctx))
	require.Equal(t, "kept", missing.Models)
}

// TestProbeConfigRevision pins which fields invalidate a probe decision: every field the
// probe sends or selects with, but not status, pricing, or usage counters.
// Parameters: t is the test handle. Returns: no values.
func TestProbeConfigRevision(t *testing.T) {
	t.Parallel()
	base := func() *Channel {
		return &Channel{Type: 1, Key: "sk", BaseURL: resetTestString("https://a"), Config: "{}", Models: "m",
			ModelMapping: resetTestString("{}"), TestingModel: resetTestString("m"), Status: ChannelStatusEnabled}
	}
	revision := base().ProbeConfigRevision()
	require.Equal(t, revision, base().ProbeConfigRevision(), "the revision is deterministic")

	for name, mutate := range map[string]func(*Channel){
		"type":          func(c *Channel) { c.Type = 2 },
		"key":           func(c *Channel) { c.Key = "sk-rotated" },
		"base url":      func(c *Channel) { c.BaseURL = resetTestString("https://b") },
		"nil base url":  func(c *Channel) { c.BaseURL = nil },
		"config":        func(c *Channel) { c.Config = `{"region":"x"}` },
		"models":        func(c *Channel) { c.Models = "m,n" },
		"model mapping": func(c *Channel) { c.ModelMapping = resetTestString(`{"a":"m"}`) },
		"testing model": func(c *Channel) { c.TestingModel = nil },
		"system prompt": func(c *Channel) { c.SystemPrompt = resetTestString("be brief") },
	} {
		changed := base()
		mutate(changed)
		require.NotEqual(t, revision, changed.ProbeConfigRevision(), name)
	}
	for name, mutate := range map[string]func(*Channel){
		"status":     func(c *Channel) { c.Status = ChannelStatusAutoDisabled },
		"used quota": func(c *Channel) { c.UsedQuota = 99 },
		"pricing":    func(c *Channel) { c.ModelConfigs = resetTestString(`{"m":{"ratio":2}}`) },
		"test time":  func(c *Channel) { c.TestTime = 5 },
	} {
		changed := base()
		mutate(changed)
		require.Equal(t, revision, changed.ProbeConfigRevision(), name)
	}
	// Field boundaries are length-prefixed, so shifting text between fields changes it.
	shifted := base()
	shifted.Key, shifted.Config = "sk{", "}"
	require.NotEqual(t, revision, shifted.ProbeConfigRevision())
}
