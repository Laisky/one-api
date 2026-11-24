package model

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/channeltype"
)

// ChannelResetCatalog is the provider knowledge a model reset needs for one
// channel type. It separates the three sources a reset must not conflate: the
// zero-config recommendations it writes, the complete provider catalog it
// validates references against, and the channel's own explicit configuration,
// which can make a non-default catalog model servable.
type ChannelResetCatalog struct {
	// Defaults are the zero-config recommendations a reset writes, in order.
	Defaults []string
	// Known is the complete provider catalog: every model the provider
	// recognizes, including models that need an operator tariff. Defaults are
	// always treated as known.
	Known []string
	// Servable reports which candidate models the channel's own explicit
	// configuration makes servable, meaning admission would not refuse them for
	// missing pricing under the channel's model_mapping and model_configs. The
	// candidates are the non-default catalog models the channel lists or
	// configures, followed by the defaults that are model_mapping keys. It
	// receives the locked, unmodified channel plus the mapping and pricing the
	// reset already decoded and validated (nil when unset, including
	// whitespace-only or null text), so the check and the validation never
	// disagree about the stored text; it must change none of them. A nil
	// Servable retains no non-default model and evaluates no default alias. It
	// returns a membership map, or an error that aborts the reset without
	// changes.
	Servable func(ctx context.Context, channel *Channel, mapping map[string]string, configs map[string]ModelConfigLocal, candidates []string) (map[string]bool, error)
}

// ChannelModelResetConflict describes a configuration that cannot safely survive
// a reset. Only field names and model identifiers are exposed, never config values.
type ChannelModelResetConflict struct {
	Code   string   `json:"code"`
	Field  string   `json:"field,omitempty"`
	Models []string `json:"models,omitempty"`
}

// Error returns a safe, actionable English description of the reset conflict.
func (conflict *ChannelModelResetConflict) Error() string {
	switch conflict.Code {
	case "unsupported_channel":
		return "Custom compatible channels cannot be reset to provider defaults. Configure their models manually."
	case "no_default_models":
		return "No default models are registered for this channel type."
	case "mapping_conflict":
		return "Model mappings reference models this provider does not support. Update the mappings before resetting."
	case "pricing_conflict":
		return "Custom pricing references models this provider does not support. Update the pricing before resetting."
	case "inference_profile_conflict":
		return "Inference profiles reference models this provider does not support. Update the profiles before resetting."
	case "tariff_incomplete":
		return "A model this channel configures needs an operator tariff that is missing or incomplete. Complete the tariff in model_configs, or change or remove the mapping or entry in the named field, before resetting."
	default:
		return "Channel configuration is invalid. Correct it before resetting models."
	}
}

// resetConflict wraps a stable conflict code, field name, and sorted model names
// for callers to classify with errors.As without exposing configuration contents.
func resetConflict(code, field string, models []string) error {
	names := slices.Clone(models)
	slices.Sort(names)
	names = slices.Compact(names)
	return errors.WithStack(&ChannelModelResetConflict{Code: code, Field: field, Models: names})
}

// decodeResetMap strictly parses an optional model-keyed JSON object. Null roots
// mean no override; null entries, malformed schemas, and untrimmed keys are refused.
func decodeResetMap[T any](raw *string, field string) (map[string]T, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" || strings.TrimSpace(*raw) == "null" {
		return nil, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*raw), &entries); err != nil {
		return nil, resetConflict("invalid_configuration", field, nil)
	}
	parsed := make(map[string]T, len(entries))
	for name, value := range entries {
		if name == "" || strings.TrimSpace(name) != name || string(value) == "null" {
			return nil, resetConflict("invalid_configuration", field, nil)
		}
		var decoded T
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, resetConflict("invalid_configuration", field, nil)
		}
		parsed[name] = decoded
	}
	return parsed, nil
}

// resetLegacyRatios is one decoded legacy per-model ratio column.
type resetLegacyRatios struct {
	field  string
	ratios map[string]float64
}

// resetOverrides holds a channel's decoded, value-validated model overrides.
type resetOverrides struct {
	mapping  map[string]string
	prices   map[string]ModelConfigLocal
	legacy   []resetLegacyRatios
	profiles map[string]string
}

// keys returns every model name the overrides configure explicitly: mapping
// keys, pricing keys, legacy ratio keys, and inference profile keys. Mapping
// targets are excluded because they need only be recognized by the provider.
func (overrides *resetOverrides) keys() []string {
	var names []string
	for name := range overrides.mapping {
		names = append(names, name)
	}
	for name := range overrides.prices {
		names = append(names, name)
	}
	for _, legacy := range overrides.legacy {
		for name := range legacy.ratios {
			names = append(names, name)
		}
	}
	for name := range overrides.profiles {
		names = append(names, name)
	}
	return names
}

// decodeResetOverrides strictly decodes and value-validates the channel's
// mappings, current and legacy pricing, inference profiles, and config without
// checking model membership. It never rewrites an override. Parameters:
// channel is the locked current row. Returns: the decoded overrides, or an
// invalid_configuration conflict naming the malformed field.
func decodeResetOverrides(channel *Channel) (*resetOverrides, error) {
	overrides := &resetOverrides{}
	var err error
	if overrides.mapping, err = decodeResetMap[string](channel.ModelMapping, "model_mapping"); err != nil {
		return nil, errors.Wrap(err, "decode reset mappings")
	}
	for _, target := range overrides.mapping {
		if strings.TrimSpace(target) == "" || strings.TrimSpace(target) != target {
			return nil, resetConflict("invalid_configuration", "model_mapping", nil)
		}
	}
	if overrides.prices, err = decodeResetMap[ModelConfigLocal](channel.ModelConfigs, "model_configs"); err != nil {
		return nil, errors.Wrap(err, "decode reset pricing")
	}
	for _, price := range overrides.prices {
		if _, err := normalizeModelConfigLocal(price); err != nil {
			return nil, resetConflict("invalid_configuration", "model_configs", nil)
		}
	}
	if err := channel.validateModelPriceConfigs(overrides.prices); err != nil {
		return nil, resetConflict("invalid_configuration", "model_configs", nil)
	}
	for _, legacy := range []struct {
		field string
		raw   *string
	}{{"model_ratio", channel.ModelRatio}, {"completion_ratio", channel.CompletionRatio}} {
		ratios, err := decodeResetMap[float64](legacy.raw, legacy.field)
		if err != nil {
			return nil, errors.Wrap(err, "decode legacy reset pricing")
		}
		for _, ratio := range ratios {
			if ratio < 0 {
				return nil, resetConflict("invalid_configuration", legacy.field, nil)
			}
		}
		overrides.legacy = append(overrides.legacy, resetLegacyRatios{field: legacy.field, ratios: ratios})
	}
	if overrides.profiles, err = decodeResetMap[string](channel.InferenceProfileArnMap, "inference_profile_arn_map"); err != nil {
		return nil, errors.Wrap(err, "decode reset inference profiles")
	}
	for _, profile := range overrides.profiles {
		if strings.TrimSpace(profile) == "" {
			return nil, resetConflict("invalid_configuration", "inference_profile_arn_map", nil)
		}
	}
	if _, err := channel.LoadConfig(); err != nil {
		return nil, resetConflict("invalid_configuration", "config", nil)
	}
	return overrides, nil
}

// resetMembershipConflict checks that every model key in entries survives the
// reset. A key the provider does not recognize at all yields code (the
// model is truly unsupported); a recognized key that the channel configures
// but cannot serve yields tariff_incomplete, because only pricing readiness
// keeps a recognized model out of the kept set. Parameters: entries are the
// field's model-keyed overrides, kept the planned model set, known the provider
// catalog, and code/field the conflict to report for unsupported keys. Returns:
// nil, or a conflict naming the offending models.
func resetMembershipConflict[T any](entries map[string]T, kept, known map[string]bool, code, field string) error {
	var unknown, incomplete []string
	for name := range entries {
		switch {
		case kept[name]:
		case known[name]:
			incomplete = append(incomplete, name)
		default:
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		return resetConflict(code, field, unknown)
	}
	if len(incomplete) > 0 {
		return resetConflict("tariff_incomplete", field, incomplete)
	}
	return nil
}

// validateResetMembership checks every override against the planned model set
// without rewriting any override. Mapping keys, pricing keys, legacy ratio
// keys, and inference profile keys must be kept, and a kept default that is a
// mapping key must also be servable through its mapping; mapping targets need
// only be recognized by the provider. Parameters: overrides are the decoded
// overrides, kept the planned model set, known the provider catalog (a
// superset of kept), and refusedAliases the kept defaults whose mapping
// admission would refuse. Returns: nil, or the first conflict in field order.
func validateResetMembership(overrides *resetOverrides, kept, known, refusedAliases map[string]bool) error {
	var unknown, incomplete []string
	for alias, target := range overrides.mapping {
		switch {
		case kept[alias] && !refusedAliases[alias]:
		case known[alias]:
			incomplete = append(incomplete, alias)
		default:
			unknown = append(unknown, alias)
		}
		if !known[target] {
			unknown = append(unknown, target)
		}
	}
	if len(unknown) > 0 {
		return resetConflict("mapping_conflict", "model_mapping", unknown)
	}
	if len(incomplete) > 0 {
		return resetConflict("tariff_incomplete", "model_mapping", incomplete)
	}
	if err := resetMembershipConflict(overrides.prices, kept, known, "pricing_conflict", "model_configs"); err != nil {
		return errors.Wrap(err, "validate reset pricing keys")
	}
	for _, legacy := range overrides.legacy {
		if err := resetMembershipConflict(legacy.ratios, kept, known, "pricing_conflict", legacy.field); err != nil {
			return errors.Wrap(err, "validate legacy reset pricing keys")
		}
	}
	if err := resetMembershipConflict(overrides.profiles, kept, known, "inference_profile_conflict", "inference_profile_arn_map"); err != nil {
		return errors.Wrap(err, "validate reset inference profile keys")
	}
	return nil
}

// resetCatalogNames trims and de-duplicates catalog names while preserving
// their first-seen order and exact case. Parameters: names is a catalog list
// and seen collects accepted names across calls. Returns: the accepted names
// and whether any non-empty name contained a comma, which a comma-separated
// model list cannot store.
func resetCatalogNames(names []string, seen map[string]bool) (accepted []string, invalid bool) {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if strings.Contains(name, ",") {
			invalid = true
			continue
		}
		seen[name] = true
		accepted = append(accepted, name)
	}
	return accepted, invalid
}

// resetRetainedModels evaluates, with one servability check over the decoded
// mapping and pricing, the non-default catalog models the channel explicitly
// lists or configures and the defaults that are model_mapping keys. A reset
// writes every default, so a default alias whose mapped target admission
// would refuse must be reported rather than written. Parameters: ctx bounds
// the servability check, channel is the locked row, overrides its decoded
// overrides, catalog the provider knowledge, defaults the accepted default
// names in order, and extras the catalog models outside the defaults, in
// catalog order. Returns: the retained non-default models in catalog order,
// the default aliases admission would refuse, or a wrapped error from
// catalog.Servable.
func resetRetainedModels(ctx context.Context, channel *Channel, overrides *resetOverrides, catalog ChannelResetCatalog, defaults, extras []string) (retained []string, refusedAliases map[string]bool, err error) {
	referenced := make(map[string]bool)
	for _, name := range channel.GetSupportedModelNames() {
		referenced[name] = true
	}
	for _, name := range overrides.keys() {
		referenced[name] = true
	}
	var candidates, aliases []string
	for _, name := range extras {
		if referenced[name] {
			candidates = append(candidates, name)
		}
	}
	for _, name := range defaults {
		if _, ok := overrides.mapping[name]; ok {
			aliases = append(aliases, name)
		}
	}
	if len(candidates)+len(aliases) == 0 || catalog.Servable == nil {
		return nil, nil, nil
	}
	servable, err := catalog.Servable(ctx, channel, overrides.mapping, overrides.prices, slices.Concat(candidates, aliases))
	if err != nil {
		return nil, nil, errors.Wrap(err, "evaluate servability of explicitly configured models")
	}
	retained = make([]string, 0, len(candidates))
	for _, name := range candidates {
		if servable[name] {
			retained = append(retained, name)
		}
	}
	for _, name := range aliases {
		if !servable[name] {
			if refusedAliases == nil {
				refusedAliases = make(map[string]bool, len(aliases))
			}
			refusedAliases[name] = true
		}
	}
	return retained, refusedAliases, nil
}

// planChannelModelReset validates a channel against its provider's catalog and
// returns a copy whose model list is the default models followed by every
// non-default catalog model the channel explicitly lists or configures and can
// serve. Overrides are never rewritten, so a reset refuses rather than silently
// dropping a model that pricing, mappings, or profiles still reference, or
// writing a default alias whose mapped target admission would refuse. The
// original row and catalog remain untouched, including on failure; unrelated
// channel settings are retained. Parameters: ctx bounds the servability check,
// channel is the locked current row, and catalog the provider knowledge.
// Returns: the planned row, the retained non-default models, or a conflict or
// operational error.
func planChannelModelReset(ctx context.Context, channel *Channel, catalog ChannelResetCatalog) (*Channel, []string, error) {
	switch channel.Type {
	case channeltype.Custom, channeltype.OpenAICompatible, channeltype.ClaudeCompatible:
		return nil, nil, resetConflict("unsupported_channel", "type", nil)
	}
	if channel.Type <= channeltype.Unknown || channel.Type >= channeltype.Dummy {
		return nil, nil, resetConflict("no_default_models", "type", nil)
	}

	defaults := make(map[string]bool, len(catalog.Defaults))
	names, invalid := resetCatalogNames(catalog.Defaults, defaults)
	// An empty Models field means unrestricted routing in legacy code. Never
	// mistake a missing catalog for a valid empty reset.
	if invalid || len(names) == 0 {
		return nil, nil, resetConflict("no_default_models", "models", nil)
	}
	// Defaults are always known; extras are the remaining catalog models in
	// catalog order, the only models a reset may retain beyond the defaults.
	known := maps.Clone(defaults)
	extras, _ := resetCatalogNames(catalog.Known, known)

	overrides, err := decodeResetOverrides(channel)
	if err != nil {
		return nil, nil, errors.Wrap(err, "validate channel before model reset")
	}
	retained, refusedAliases, err := resetRetainedModels(ctx, channel, overrides, catalog, names, extras)
	if err != nil {
		return nil, nil, errors.Wrap(err, "plan retained models")
	}
	kept := maps.Clone(defaults)
	for _, name := range retained {
		kept[name] = true
	}
	if err := validateResetMembership(overrides, kept, known, refusedAliases); err != nil {
		return nil, nil, errors.Wrap(err, "validate channel before model reset")
	}

	planned := *channel
	planned.Models = strings.Join(append(names, retained...), ",")
	planned.HiddenModels = nil
	// The shared policy keeps SKIP (in any legacy spelling) and still-listed models,
	// and reverts a selection the reset removes to auto-select.
	planned.TestingModel = planned.NormalizeTestingModel(planned.TestingModel)
	return &planned, retained, nil
}
