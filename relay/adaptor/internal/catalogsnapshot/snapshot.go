// Package catalogsnapshot applies audited, embedded provider catalog patches.
// It performs no network or account-dependent discovery at runtime.
package catalogsnapshot

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Source identifies the exact official response used to generate a snapshot.
type Source struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// Snapshot stores partial model patches; token prices are native currency per
// million tokens, not quota units. Missing fields preserve existing metadata.
type Snapshot struct {
	Version  int                        `json:"version"`
	Currency string                     `json:"currency"`
	Sources  []Source                   `json:"sources"`
	Models   map[string]json.RawMessage `json:"models"`
}

// Apply returns an independent catalog with raw's audited patches applied to
// base. It panics on invalid checked-in data, before exposing partial results.
// Historical IDs absent from the snapshot are retained without repricing.
func Apply(base map[string]adaptor.ModelConfig, raw []byte) map[string]adaptor.ModelConfig {
	result, err := Decode(base, raw)
	if err != nil {
		panic(err)
	}
	return result
}

// Decode validates raw and returns a deep copy of base with explicit snapshot
// fields replaced. It returns an error without mutating base on invalid data.
func Decode(base map[string]adaptor.ModelConfig, raw []byte) (map[string]adaptor.ModelConfig, error) {
	var snapshot Snapshot
	if err := decodeStrict(raw, &snapshot); err != nil {
		return nil, errors.Wrap(err, "decode catalog snapshot")
	}
	if snapshot.Version != 1 || len(snapshot.Sources) == 0 {
		return nil, errors.New("catalog snapshot requires version 1 and sources")
	}
	factor := ratio.MilliTokensUsd
	switch snapshot.Currency {
	case "USD":
	case "CNY":
		factor = ratio.MilliTokensRmb
	default:
		return nil, errors.New("unsupported catalog currency")
	}
	// Avoid adding independently sized map lengths in an allocation hint.
	// Growth handles new model IDs, and overlapping IDs need no extra capacity.
	result := make(map[string]adaptor.ModelConfig, len(base))
	for id, config := range base {
		result[id] = config.Clone()
	}

	for id, patch := range snapshot.Models {
		if id == "" || id != strings.TrimSpace(id) {
			return nil, errors.New("invalid catalog model ID")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(patch, &fields); err != nil {
			return nil, errors.Wrap(err, "decode catalog fields")
		}
		if fields == nil {
			return nil, errors.New("null catalog model patch")
		}
		if _, ok := fields["max_tokens"]; ok {
			return nil, errors.New("catalog updates must not set request caps")
		}
		_, existing := base[id]
		if _, priced := fields["ratio"]; !existing && !priced {
			return nil, errors.New("new catalog model requires an explicit price")
		}
		config := result[id]
		// Some source tables state affirmative capabilities only. Merge those
		// without pretending an omitted capability has been removed upstream.
		var additions []string
		if rawFeatures, ok := fields["add_supported_features"]; ok {
			if err := decodeStrict(rawFeatures, &additions); err != nil {
				return nil, errors.Wrap(err, "decode additive capabilities")
			}
			delete(fields, "add_supported_features")
			patch, _ = json.Marshal(fields) // RawMessage values were validated above.
		}
		if _, supplied := fields["completion_ratio"]; !existing && !supplied {
			return nil, errors.New("new catalog model requires an explicit output price")
		}
		// Supplied schedules replace the entire array, not individual fields of
		// reused elements. Otherwise omitted prices/dates survive from base and
		// already-normalized quota prices are scaled a second time below.
		if _, supplied := fields["tiers"]; supplied {
			config.Tiers = nil
		}
		if _, supplied := fields["time_windows"]; supplied {
			config.TimeWindows = nil
		}
		if err := decodeStrict(patch, &config); err != nil {
			return nil, errors.Wrap(err, "decode model metadata")
		}
		// Scale only fields explicitly supplied by the snapshot. Preserved quota
		// fields are already normalized and must not be converted a second time.
		for key, target := range map[string]*float64{
			"ratio": &config.Ratio, "cached_input_ratio": &config.CachedInputRatio,
			"cache_write_5m_ratio": &config.CacheWrite5mRatio, "cache_write_1h_ratio": &config.CacheWrite1hRatio,
		} {
			if _, supplied := fields[key]; supplied && *target > 0 {
				*target *= factor
			}
		}
		if _, supplied := fields["tiers"]; supplied {
			for i := range config.Tiers {
				tier := &config.Tiers[i]
				tier.Ratio *= factor
				if tier.CachedInputRatio > 0 {
					tier.CachedInputRatio *= factor
				}
				if tier.CacheWrite5mRatio > 0 {
					tier.CacheWrite5mRatio *= factor
				}
				if tier.CacheWrite1hRatio > 0 {
					tier.CacheWrite1hRatio *= factor
				}
			}
		}
		if _, supplied := fields["time_windows"]; supplied {
			for i := range config.TimeWindows {
				window := &config.TimeWindows[i].Overlay
				window.Ratio *= factor
				if window.CachedInputRatio > 0 {
					window.CachedInputRatio *= factor
				}
				if window.CacheWrite5mRatio > 0 {
					window.CacheWrite5mRatio *= factor
				}
				if window.CacheWrite1hRatio > 0 {
					window.CacheWrite1hRatio *= factor
				}
			}
		}
		if embedding, supplied := fields["embedding"]; supplied && config.Embedding != nil {
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(embedding, &nested); err != nil {
				return nil, errors.Wrap(err, "decode embedding tariff")
			}
			if _, supplied := nested["text_token_ratio"]; supplied {
				config.Embedding.TextTokenRatio *= factor
			}
			if _, supplied := nested["image_token_ratio"]; supplied {
				config.Embedding.ImageTokenRatio *= factor
			}
		}
		if config.Ratio < 0 || config.CompletionRatio < 0 || math.IsInf(config.Ratio*config.CompletionRatio, 0) {
			return nil, errors.New("invalid catalog token price")
		}
		for _, feature := range additions {
			if feature == "" || feature != strings.TrimSpace(feature) {
				return nil, errors.New("invalid additive capability")
			}
			if !slices.Contains(config.SupportedFeatures, feature) {
				config.SupportedFeatures = append(config.SupportedFeatures, feature)
			}
		}
		result[id] = config
	}
	return result, nil
}

// decodeStrict decodes exactly one JSON value from raw into out, rejecting
// misspelled fields and trailing values. It returns the decoding error, if any.
func decodeStrict(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return errors.Wrap(err, "decode JSON")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing catalog JSON")
	}
	return nil
}
