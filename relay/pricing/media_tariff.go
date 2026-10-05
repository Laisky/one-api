package pricing

import (
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ValidateTariffProvenance rejects explicitly unresolved or expired catalog
// contracts. Parameters: cfg is the effective tariff and at is the request UTC
// time. Returns: an error before dispatch, or nil for legacy/verified contracts.
func ValidateTariffProvenance(cfg adaptor.ModelConfig, at time.Time) error {
	p := cfg.PricingProvenance
	if p == nil {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if !p.ValidUntil.IsZero() && !at.Before(p.ValidUntil) {
		return errors.New("media tariff expired; configure an explicit operator tariff")
	}
	switch p.State {
	case "free", "promotional_free", "paid":
		if p.Source == "" || p.VerifiedAt == "" || p.Unit == "" {
			return errors.New("media tariff provenance is incomplete")
		}
		if p.State == "free" || p.State == "promotional_free" {
			if cfg.Ratio != 0 || (cfg.PerCall != nil && cfg.PerCall.UsdPerThousandCalls != 0) || (cfg.Audio != nil && (cfg.Audio.InputPriceUsd != 0 || cfg.Audio.UsdPerSecond != 0)) {
				return errors.New("free media provenance conflicts with a paid tariff")
			}
		}
		if p.State == "paid" {
			switch p.Unit {
			case "generation":
				if cfg.PerCall == nil || cfg.PerCall.UsdPerThousandCalls <= 0 {
					return errors.New("paid generation tariff is missing")
				}
			case "characters":
				if cfg.Audio == nil || cfg.Audio.InputUnit != "characters" || cfg.Audio.InputPriceQuantity <= 0 || cfg.Audio.InputPriceUsd <= 0 {
					return errors.New("paid character tariff is missing")
				}
			default:
				return errors.New("media tariff unit requires an explicit operator contract")
			}
		}
	case "unknown", "contract":
		return errors.New("media tariff requires an explicit operator contract")
	default:
		return errors.New("unrecognized media tariff state")
	}
	return nil
}

// ResolveGenerationTariff resolves single-generation prices for models whose
// provider catalog explicitly declares that billing unit. Parameters: name,
// overrides, provider and at select the normal pricing layers. Returns: the flat
// tariff, whether the generation contract applies, and an error if unresolved.
// Explicit per-call zero and ratio-zero/completion overrides preserve free policy.
func ResolveGenerationTariff(name string, overrides map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (*adaptor.PerCallPricingConfig, bool, error) {
	base, known := ResolveModelConfig(name, nil, provider, at)
	if !known || base.PricingProvenance == nil || base.PricingProvenance.Unit != "generation" {
		return nil, false, nil
	}
	cfg, _ := ResolveModelConfig(name, overrides, provider, at)
	if err := ValidateTariffProvenance(cfg, at); err != nil {
		return nil, true, err
	}
	if cfg.PerCall != nil {
		return cfg.PerCall, true, nil
	}
	if local, ok := overrides[name]; ok && local.Ratio == 0 && local.CompletionRatio > 0 && local.Audio == nil && len(local.Tiers) == 0 && len(local.TimeWindows) == 0 {
		return &adaptor.PerCallPricingConfig{}, true, nil
	}
	return nil, true, errors.New("generation billing requires per_call pricing; token ratios cannot price songs")
}

// GenerationQuota prices one generation using exact decimal arithmetic.
// Parameters: tariff is USD per thousand generations and group is the operator
// multiplier. Returns: a quota rounded up once, or an error for invalid/overflowing
// prices. A verified free tariff or explicit free group returns zero.
func GenerationQuota(tariff *adaptor.PerCallPricingConfig, group float64) (int64, error) {
	if tariff == nil {
		return 0, errors.New("generation tariff is missing")
	}
	cost := big.NewRat(ratio.QuotaPerUsd, 1000)
	for _, value := range []float64{tariff.UsdPerThousandCalls, group} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, errors.New("generation tariff must be finite and nonnegative")
		}
		decimal, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'f', -1, 64))
		if !ok {
			return 0, errors.New("invalid generation tariff decimal")
		}
		cost.Mul(cost, decimal)
	}
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(cost.Num(), cost.Denom(), remainder)
	if remainder.Sign() != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, errors.New("generation tariff exceeds quota range")
	}
	return quotient.Int64(), nil
}
