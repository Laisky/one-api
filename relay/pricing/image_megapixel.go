package pricing

import (
	"math"
	"math/big"
	"strconv"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/adaptor"
)

// maxImageMegapixelSide bounds one output side accepted for megapixel pricing.
// It only rejects absurd values before arithmetic; providers enforce their own,
// much smaller, limits.
const maxImageMegapixelSide = 1 << 20

// ImageDimensions describes one image measured for megapixel pricing.
// Known is false when the dimensions could not be determined, in which case
// Width and Height are ignored and the tariff's conservative fallback applies.
type ImageDimensions struct {
	Width  int
	Height int
	Known  bool
}

// ImageMegapixelQuote is the price of one generated image under an opt-in
// megapixel tariff, with the billed quantities that produced it.
type ImageMegapixelQuote struct {
	// OutputMegapixels is the billed output megapixel count after rounding.
	OutputMegapixels float64
	// ReferenceMegapixels is the billed megapixel count summed over every
	// reference image after rounding, including fallback estimates.
	ReferenceMegapixels float64
	// References is the number of reference images priced.
	References int
	// UnknownReferences is how many references were billed at the fallback
	// because their dimensions could not be decoded.
	UnknownReferences int
	// UnitUsd is the USD price of one generated image, rounded to the nearest
	// float64 of the exact decimal sum.
	UnitUsd float64
}

// ValidateImageMegapixelPricing checks that a megapixel tariff can be billed.
// Parameters: cfg is the tariff and must not be nil. Returns: nil when every
// price is finite and non-negative, the first megapixel is priced, the rounding
// rule is known, and a reference price has a positive fallback; otherwise an error.
func ValidateImageMegapixelPricing(cfg *adaptor.ImageMegapixelPricing) error {
	if cfg == nil {
		return errors.New("image megapixel tariff is missing")
	}
	for _, price := range []float64{cfg.FirstMegapixelUsd, cfg.AdditionalMegapixelUsd, cfg.ReferenceMegapixelUsd} {
		if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
			return errors.New("image megapixel prices must be finite and non-negative")
		}
	}
	if cfg.FirstMegapixelUsd <= 0 {
		return errors.New("image megapixel tariff must price the first megapixel")
	}
	if cfg.PixelsPerMegapixel < 0 || cfg.OutputDimensionStep < 0 || cfg.ReferenceFallbackMegapixels < 0 {
		return errors.New("image megapixel tariff sizes must be non-negative")
	}
	switch cfg.Rounding {
	case "", adaptor.ImageMegapixelRoundingCeil, adaptor.ImageMegapixelRoundingExact:
	default:
		return errors.Errorf("unsupported image megapixel rounding %q", cfg.Rounding)
	}
	if cfg.ReferenceMegapixelUsd > 0 && cfg.ReferenceFallbackMegapixels <= 0 {
		return errors.New("image megapixel tariff with a reference price needs reference_fallback_megapixels")
	}
	return nil
}

// ImageMegapixelQuoteOptions carries host billing rules applied on top of a
// megapixel tariff. The zero value applies the tariff as configured.
type ImageMegapixelQuoteOptions struct {
	// ProRataOutput bills an output smaller than one megapixel its exact
	// fraction of FirstMegapixelUsd instead of the full first megapixel, for a
	// host that publishes a true per-megapixel rate. It has no effect under
	// ceil rounding, which never bills less than one megapixel.
	ProRataOutput bool
}

// QuoteImageMegapixels prices one generated image under a megapixel tariff.
// Parameters: cfg is the tariff, outputWidth and outputHeight are the requested
// output size in pixels, and references lists every reference image sent with
// the request. Returns: the per-image quote, or an error for an invalid tariff
// or output size. All arithmetic is exact; only UnitUsd is converted to float64.
func QuoteImageMegapixels(cfg *adaptor.ImageMegapixelPricing, outputWidth, outputHeight int, references []ImageDimensions) (ImageMegapixelQuote, error) {
	return QuoteImageMegapixelsWithOptions(cfg, outputWidth, outputHeight, references, ImageMegapixelQuoteOptions{})
}

// QuoteImageMegapixelsWithOptions is QuoteImageMegapixels with host billing
// rules. Parameters: cfg, outputWidth, outputHeight and references are as for
// QuoteImageMegapixels, and opts selects the host rules. Returns: the
// per-image quote, or an error for an invalid tariff or output size.
func QuoteImageMegapixelsWithOptions(cfg *adaptor.ImageMegapixelPricing, outputWidth, outputHeight int, references []ImageDimensions, opts ImageMegapixelQuoteOptions) (ImageMegapixelQuote, error) {
	if err := ValidateImageMegapixelPricing(cfg); err != nil {
		return ImageMegapixelQuote{}, err
	}
	if outputWidth <= 0 || outputHeight <= 0 || outputWidth > maxImageMegapixelSide || outputHeight > maxImageMegapixelSide {
		return ImageMegapixelQuote{}, errors.Errorf("image output size %dx%d is out of range", outputWidth, outputHeight)
	}
	perMegapixel := cfg.PixelsPerMegapixel
	if perMegapixel == 0 {
		perMegapixel = adaptor.DefaultPixelsPerMegapixel
	}
	exact := cfg.Rounding == adaptor.ImageMegapixelRoundingExact
	first, err := decimalRat(cfg.FirstMegapixelUsd)
	if err != nil {
		return ImageMegapixelQuote{}, err
	}
	additional, err := decimalRat(cfg.AdditionalMegapixelUsd)
	if err != nil {
		return ImageMegapixelQuote{}, err
	}
	referencePrice, err := decimalRat(cfg.ReferenceMegapixelUsd)
	if err != nil {
		return ImageMegapixelQuote{}, err
	}

	width := roundUpToStep(int64(outputWidth), int64(cfg.OutputDimensionStep))
	height := roundUpToStep(int64(outputHeight), int64(cfg.OutputDimensionStep))
	output := billedMegapixels(width*height, perMegapixel, exact)
	// The first megapixel is a minimum: a smaller output still costs it,
	// unless the host bills a true rate pro rata.
	one := big.NewRat(1, 1)
	extra := new(big.Rat).Sub(output, one)
	if extra.Sign() < 0 {
		extra.SetInt64(0)
	}
	firstShare := first
	if opts.ProRataOutput && output.Cmp(one) < 0 {
		firstShare = new(big.Rat).Mul(first, output)
	}
	unit := new(big.Rat).Add(firstShare, new(big.Rat).Mul(additional, extra))

	quote := ImageMegapixelQuote{References: len(references)}
	referenceTotal := new(big.Rat)
	for _, reference := range references {
		if !reference.Known || reference.Width <= 0 || reference.Height <= 0 {
			quote.UnknownReferences++
			referenceTotal.Add(referenceTotal, big.NewRat(int64(cfg.ReferenceFallbackMegapixels), 1))
			continue
		}
		pixels := int64(reference.Width) * int64(reference.Height)
		referenceTotal.Add(referenceTotal, billedMegapixels(pixels, perMegapixel, exact))
	}
	unit.Add(unit, new(big.Rat).Mul(referencePrice, referenceTotal))

	quote.OutputMegapixels, _ = output.Float64()
	quote.ReferenceMegapixels, _ = referenceTotal.Float64()
	quote.UnitUsd, _ = unit.Float64()
	return quote, nil
}

// billedMegapixels converts a pixel count into billed megapixels.
// Parameters: pixels is the image area, perMegapixel defines one megapixel, and
// exact selects pro-rata billing. Returns: the exact megapixel ratio when exact,
// otherwise the ratio rounded up to the next whole megapixel.
func billedMegapixels(pixels, perMegapixel int64, exact bool) *big.Rat {
	ratio := big.NewRat(pixels, perMegapixel)
	if exact || ratio.IsInt() {
		return ratio
	}
	whole := new(big.Int).Quo(ratio.Num(), ratio.Denom())
	return new(big.Rat).SetInt(whole.Add(whole, big.NewInt(1)))
}

// roundUpToStep rounds value up to a multiple of step.
// Parameters: value is a positive side length and step is the grid, where a
// non-positive step disables rounding. Returns: the rounded side length.
func roundUpToStep(value, step int64) int64 {
	if step <= 0 || value%step == 0 {
		return value
	}
	return (value/step + 1) * step
}

// decimalRat converts a configured USD price to an exact rational using its
// shortest decimal representation, so 0.07 is 7/100 rather than its binary
// approximation. Parameters: value is a finite price. Returns: the rational or
// an error when the value cannot be represented.
func decimalRat(value float64) (*big.Rat, error) {
	rational, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'f', -1, 64))
	if !ok {
		return nil, errors.Errorf("invalid image megapixel price %v", value)
	}
	return rational, nil
}
