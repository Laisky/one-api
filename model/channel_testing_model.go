package model

import "strings"

// ChannelTestingModelField is the JSON field and database column that stores a
// channel's health-check model selection.
const ChannelTestingModelField = "testing_model"

// IsChannelTestingModelSkip reports whether value spells the SKIP sentinel.
//
// The comparison ignores surrounding whitespace and letter case, matching how the
// health checker has always interpreted the column, so legacy spellings such as
// "__SKIP__" keep opting their channel out of testing.
//
// Parameters:
//   - value: a raw testing_model value from a request payload or the database.
//
// Return values:
//   - bool: true when value selects SKIP rather than naming a model.
func IsChannelTestingModelSkip(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), ChannelTestingModelSkip)
}

// NormalizeChannelTestingModel applies the single testing_model storage policy shared by
// channel creation, updates, duplication, and model resets.
//
// The column holds exactly one of three states:
//   - nil: auto-select the cheapest chat-format model at test time.
//   - ChannelTestingModelSkip: never health-check the channel. Every accepted spelling
//     is stored as the canonical sentinel.
//   - a model name present in the channel model list, trimmed of whitespace.
//
// A blank value, or a model name the channel does not list, resolves to nil so a stale
// selection can never point the health check at a model the channel cannot serve.
//
// Parameters:
//   - value: the requested or previously stored testing_model; nil means auto-select.
//   - models: the comma-separated model list the selection is validated against.
//
// Return values:
//   - *string: a newly allocated normalized value, or nil for auto-select.
func NormalizeChannelTestingModel(value *string, models string) *string {
	if value == nil {
		return nil
	}
	name := strings.TrimSpace(*value)
	switch {
	case name == "":
		return nil
	case IsChannelTestingModelSkip(name):
		skip := ChannelTestingModelSkip
		return &skip
	case channelModelListContains(models, name):
		return &name
	default:
		return nil
	}
}

// IsTestingSkipped reports whether an administrator excluded the channel from health
// checking by selecting SKIP as its testing model.
//
// Parameters: none (pointer receiver; a nil channel is never skipped).
//
// Return values:
//   - bool: true when the channel must not be probed automatically.
func (channel *Channel) IsTestingSkipped() bool {
	return channel != nil && channel.TestingModel != nil && IsChannelTestingModelSkip(*channel.TestingModel)
}

// resolveUpdatedTestingModel decides the testing_model an update must persist.
//
// Updates follow JSON Merge Patch (RFC 7396) semantics for this field: an omitted
// field keeps the stored selection, while an explicit null or blank string clears it
// back to auto-select. Either way the result is normalized against the effective
// model list, so a selection the new list no longer serves falls back to auto-select
// and SKIP, which is not a model name, always survives.
//
// Parameters:
//   - requested: the testing_model carried by the update payload.
//   - provided: whether the payload carried the field at all, including as null.
//   - stored: the testing_model currently persisted for the channel.
//   - models: the effective comma-separated model list after the update.
//
// Return values:
//   - *string: the normalized value to persist, or nil for auto-select.
func resolveUpdatedTestingModel(requested *string, provided bool, stored *string, models string) *string {
	if provided {
		return NormalizeChannelTestingModel(requested, models)
	}
	return NormalizeChannelTestingModel(stored, models)
}

// canonicalChannelTestingModel reports a stored testing_model the way the health
// checker interprets it, replacing a legacy SKIP spelling with the canonical sentinel.
//
// It is applied on read so administrators see the same opt-out the checker honours
// without rewriting rows; every other value, valid or stale, is returned unchanged.
//
// Parameters:
//   - value: the persisted testing_model.
//
// Return values:
//   - *string: value itself, or a pointer to ChannelTestingModelSkip for a SKIP spelling.
func canonicalChannelTestingModel(value *string) *string {
	if value == nil || *value == ChannelTestingModelSkip || !IsChannelTestingModelSkip(*value) {
		return value
	}
	skip := ChannelTestingModelSkip
	return &skip
}

// sameOptionalString reports whether two optional strings hold the same value.
//
// Parameters:
//   - a, b: the optional values to compare; two nil pointers are equal.
//
// Return values:
//   - bool: true when both are nil or both point at equal strings.
func sameOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// channelModelListContains reports whether a comma-separated channel model list
// contains name after trimming each entry.
//
// Parameters:
//   - models: the comma-separated channel model list.
//   - name: the trimmed model name to look for.
//
// Return values:
//   - bool: true when an entry equals name exactly.
func channelModelListContains(models, name string) bool {
	for entry := range strings.SplitSeq(models, ",") {
		if strings.TrimSpace(entry) == name {
			return true
		}
	}
	return false
}
