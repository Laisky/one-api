package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// ChannelTestingModelField is the JSON field and database column that stores a
// channel's health-check model selection.
const ChannelTestingModelField = "testing_model"

// IsChannelTestingModelSkip reports whether value spells the SKIP sentinel.
// The comparison ignores surrounding whitespace and letter case, matching how the
// health checker has always read the column, so legacy spellings such as "__SKIP__"
// keep opting their channel out.
//
// Parameters:
//   - value: a raw testing_model value from a request payload or the database.
//
// Return values:
//   - bool: true when value selects SKIP rather than naming a model.
func IsChannelTestingModelSkip(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), ChannelTestingModelSkip)
}

// NormalizeTestingModel applies the single testing_model storage policy shared by
// channel creation, updates, duplication, and model resets. The result is one of:
//   - nil: auto-select the cheapest chat-format model at test time;
//   - ChannelTestingModelSkip: never health-check the channel (any accepted spelling
//     is stored canonically);
//   - a trimmed model name the channel serves, by the same SupportsModel rule the health
//     checker applies before honouring a stored selection. That includes any model on a
//     channel with an empty (unrestricted) model list and mapping aliases whose target
//     the channel lists, so the selector, the persisted value, and the probe agree.
//
// A blank value, or a model the channel cannot serve, resolves to nil.
//
// Parameters:
//   - value: the requested or previously stored testing_model; nil means auto-select.
//
// Return values:
//   - *string: a newly allocated normalized value, or nil for auto-select.
func (channel *Channel) NormalizeTestingModel(value *string) *string {
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
	case channel.SupportsModel(name):
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

// resolveTestingModelUpdate decides the testing_model an update must persist, following
// JSON Merge Patch (RFC 7396) semantics: an omitted field keeps the stored selection and
// an explicit null or blank clears it to auto-select. Either way the result is
// normalized against the channel as it stands after the update, so a selection the new
// model list no longer serves reverts to auto-select while SKIP always survives.
//
// Parameters:
//   - current: the channel row after this update's other columns were written; its
//     TestingModel is the latest stored selection.
//   - requested: the testing_model carried by the update.
//   - provided: whether the update carried the field at all, including as null.
//
// Return values:
//   - *string: the normalized value to persist, or nil for auto-select.
func resolveTestingModelUpdate(current *Channel, requested *string, provided bool) *string {
	if provided {
		return current.NormalizeTestingModel(requested)
	}
	return current.NormalizeTestingModel(current.TestingModel)
}

// testingModelResolution records what an update did to testing_model, for diagnostics.
type testingModelResolution struct {
	previous  *string
	persisted *string
	written   bool
}

// persistResolvedTestingModel resolves and stores testing_model inside an update
// transaction that already holds the channel row's write lock.
//
// The selection is read from the locked row rather than from a snapshot taken before
// the transaction, and it is written only through this explicit column update, never
// by the general struct update. An update that omits the field therefore cannot
// re-assert a selection that a concurrent request replaced.
//
// Parameters:
//   - tx: the update transaction holding the channel row lock.
//   - id: the channel being updated.
//   - requested: the testing_model carried by the update.
//   - provided: whether the update carried the field at all.
//
// Return values:
//   - testingModelResolution: the previous value, the persisted value, and whether it changed.
//   - error: a wrapped read or write failure.
func persistResolvedTestingModel(tx *gorm.DB, id int, requested *string, provided bool) (testingModelResolution, error) {
	var current Channel
	if err := tx.Select("id", "models", "model_mapping", ChannelTestingModelField).
		First(&current, "id = ?", id).Error; err != nil {
		return testingModelResolution{}, errors.Wrapf(err, "read locked testing_model for channel %d", id)
	}
	resolution := testingModelResolution{
		previous:  current.TestingModel,
		persisted: resolveTestingModelUpdate(&current, requested, provided),
	}
	if sameOptionalString(resolution.persisted, current.TestingModel) {
		return resolution, nil
	}
	if err := tx.Model(&Channel{}).Where("id = ?", id).
		Update(ChannelTestingModelField, resolution.persisted).Error; err != nil {
		return testingModelResolution{}, errors.Wrapf(err, "update testing_model for channel %d", id)
	}
	resolution.written = true
	return resolution, nil
}

// probeConfigColumns are the channel columns that determine what a health-check probe
// sends and how its testing model is chosen. ProbeConfigRevision covers exactly these.
var probeConfigColumns = []string{
	"id", "type", "key", "base_url", "other", "config", "models", "model_mapping",
	"system_prompt", "inference_profile_arn_map", ChannelTestingModelField,
}

// ProbeConfigRevision returns a digest of the configuration a health-check probe depends
// on: type, key, base URL, legacy and current config, models, model mapping, system
// prompt, inference profile map, and testing model. A result or cleanup decided from one
// revision must not be applied once the stored channel has another, because it no longer
// describes the channel. Status is deliberately excluded; it is compared separately.
// The digest is only compared in memory and never logged.
//
// Other columns the probe's request context reads are excluded because they cannot change
// what the upstream receives or how it answers: group only feeds the channel billing
// ratio (and the test cost ignores it), ratelimit is enforced by the relay middleware,
// which the probe bypasses, and model_configs or the legacy ratios only rank which
// served model auto-selection probes, while a probe of any served model remains a valid
// health sample. Name, weight, priority, and usage counters never reach the probe.
//
// Parameters: none (pointer receiver).
//
// Return values:
//   - string: a hex SHA-256 digest of the probe-relevant fields.
func (channel *Channel) ProbeConfigRevision() string {
	digest := sha256.New()
	writeField := func(value string) {
		_, _ = fmt.Fprintf(digest, "%d:%s|", len(value), value)
	}
	writeOptional := func(value *string) {
		if value == nil {
			_, _ = digest.Write([]byte("-|"))
			return
		}
		writeField(*value)
	}
	writeField(strconv.Itoa(channel.Type))
	writeField(channel.Key)
	writeOptional(channel.BaseURL)
	writeOptional(channel.Other)
	writeField(channel.Config)
	writeField(channel.Models)
	writeOptional(channel.ModelMapping)
	writeOptional(channel.SystemPrompt)
	writeOptional(channel.InferenceProfileArnMap)
	writeOptional(channel.TestingModel)
	return hex.EncodeToString(digest.Sum(nil))
}

// ReloadForProbeWithContext replaces the channel with its current stored row, so a
// long-running health-check sweep chooses and probes from a consistent, current
// configuration instead of the snapshot it loaded at start.
//
// Parameters:
//   - ctx: sweep scope for the read.
//
// Return values:
//   - error: a wrapped database failure; the channel is unchanged on error.
func (channel *Channel) ReloadForProbeWithContext(ctx context.Context) error {
	var current Channel
	if err := DB.WithContext(ctx).First(&current, "id = ?", channel.Id).Error; err != nil {
		return errors.Wrapf(err, "reload channel %d for probe", channel.Id)
	}
	*channel = current
	return nil
}

// ClearStaleChannelTestingModel reverts a stored testing model that the health checker
// found unusable back to auto-select, but only if the channel still has the exact probe
// configuration the checker judged it against (including the testing model itself). A
// selection saved since, or a model list or mapping changed since, is left untouched.
//
// Parameters:
//   - ctx: request or sweep scope for the transaction.
//   - id: the channel whose selection was found stale.
//   - revision: the ProbeConfigRevision the checker based its decision on.
//
// Return values:
//   - bool: true when the stale value was cleared, false when the channel had changed.
//   - error: a wrapped database failure.
func ClearStaleChannelTestingModel(ctx context.Context, id int, revision string) (bool, error) {
	cleared := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockChannelRow(tx, id); err != nil {
			return errors.Wrap(err, "lock channel for stale testing_model cleanup")
		}
		var current Channel
		if err := tx.Select(probeConfigColumns).First(&current, "id = ?", id).Error; err != nil {
			return errors.Wrap(err, "read locked channel for stale testing_model cleanup")
		}
		if current.TestingModel == nil || current.ProbeConfigRevision() != revision {
			return nil
		}
		if err := tx.Model(&Channel{}).Where("id = ?", id).Update(ChannelTestingModelField, nil).Error; err != nil {
			return errors.Wrap(err, "clear stale testing_model")
		}
		cleared = true
		return nil
	})
	if err != nil {
		return false, errors.Wrapf(err, "clear stale testing_model for channel %d", id)
	}
	return cleared, nil
}

// canonicalChannelTestingModel reports a stored testing_model the way the health
// checker interprets it: a legacy SKIP spelling becomes the canonical sentinel, and
// every other value is returned unchanged. It is applied on read and never rewrites rows.
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
