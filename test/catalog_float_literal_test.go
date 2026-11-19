package test

import (
	"go/scanner"
	"go/token"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// catalogLiteralRoot is the adaptor tree whose catalog files the guard scans,
// relative to this package.
const catalogLiteralRoot = "../relay/adaptor"

// catalogLiteralMaxULP is how far, in units in the last place, a literal may be
// from a short decimal and still count as that decimal's float-conversion artifact.
const catalogLiteralMaxULP = 4

// catalogLiteralFileRe selects hand-written catalog data files: models*.go,
// constants*.go, *catalog*.go and pricing_*.go. Test files are excluded separately.
var catalogLiteralFileRe = regexp.MustCompile(`^(models.*|constants.*|.*catalog.*|pricing_.*)\.go$`)

// catalogLiteralLongFractionRe matches a mantissa with 14 or more fractional digits.
var catalogLiteralLongFractionRe = regexp.MustCompile(`\.[0-9]{14,}`)

// catalogLiteralPrintedExponentRe matches the exponent strconv.FormatFloat and
// fmt's %g emit (an explicit sign and at least two digits, as in 1e+01 or
// 7.5e-06). A hand-written scale such as InputPriceQuantity: 1e6 does not match.
var catalogLiteralPrintedExponentRe = regexp.MustCompile(`[eE][+-][0-9]{2,}$`)

// grandfatheredCatalogFloatLiterals lists the noisy literals that existed when the
// guard was introduced and have no short decimal or exact-quotient form: repeating
// quotients (0.001 / 60 per second) and 15-significant-digit truncations copied
// from provider APIs. Keys are "<path below relay/adaptor><TAB><literal>", values
// the allowed occurrence count. The list may only shrink: rewriting one of these
// as an exact expression (for example `0.0002 / 60`) requires deleting its entry.
var grandfatheredCatalogFloatLiterals = map[string]int{
	"deepinfra/models_mistralai.go\t1.6666666666666667":       1,
	"deepinfra/models_mistralai.go\t1.6666666666666667e-05":   1,
	"deepinfra/models_nvidia.go\t0.3333333333333333":          1,
	"deepinfra/models_nvidia.go\t3.3333333333333333e-06":      1,
	"deepinfra/models_openai.go\t0.3333333333333333":          1,
	"deepinfra/models_openai.go\t3.3333333333333333e-06":      1,
	"deepinfra/models_qwen.go\t0.3333333333333333":            1,
	"deepinfra/models_qwen.go\t3.3333333333333333e-06":        1,
	"novita/models_tencent.go\t2.99880095923261":              1,
	"novita/models_tencent.go\t4.01428571428571":              1,
	"openai/models_audio.go\t28.333333333333333":              1,
	"openrouter/models_baidu.go\t3.48672566371681":            1,
	"openrouter/models_deepseek.go\t1.46153846153846":         1,
	"openrouter/models_deepseek.go\t1.66666666666667":         1,
	"openrouter/models_google.go\t0.183333333333333":          1,
	"openrouter/models_google.go\t0.333333333333333":          2,
	"openrouter/models_google.go\t0.383333333333333":          2,
	"openrouter/models_google.go\t0.583333333333333":          1,
	"openrouter/models_google.go\t0.791666666666667":          2,
	"openrouter/models_google.go\t0.866666666666667":          1,
	"openrouter/models_google.go\t1.58333333333333":           3,
	"openrouter/models_google.go\t3.33333333333333":           1,
	"openrouter/models_meta_llama.go\t2.85714285714286":       1,
	"openrouter/models_nex_agi.go\t3.33333333333333":          1,
	"openrouter/models_nvidia.go\t3.33333333333333":           1,
	"openrouter/models_other.go\t1.66666666666667":            2,
	"openrouter/models_other.go\t3.33333333333333":            1,
	"openrouter/models_qwen.go\t5.71428571428571":             1,
	"openrouter/models_tencent.go\t2.99880095923261":          1,
	"openrouter/models_thinkingmachines.go\t3.33333333333333": 1,
	"togetherai/models_nvidia.go\t3.33333333333333":           1,
	"togetherai/models_other.go\t3.33333333333333":            1,
}

// TestCatalogFloatLiteralsAreWrittenAsPublishedDecimals rejects float-conversion
// artifacts in hand-written catalog files, such as nativeRate(0.7999999999999999),
// nativeRate(1e+01) or CompletionRatio: 3.9285714285714293.
//
// Such literals are printouts of float64 arithmetic on a provider's published
// decimal price. They obscure the price a reviewer must check against the
// provider page and invite copy-paste drift. A literal is rejected when its
// mantissa has 14 or more fractional digits, or when it uses the printed
// exponent form (1e+01, 7.5e-06) although its value has a short decimal form.
func TestCatalogFloatLiteralsAreWrittenAsPublishedDecimals(t *testing.T) {
	found := map[string]int{}
	locations := map[string][]string{}
	scanned := 0
	err := filepath.WalkDir(catalogLiteralRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() || strings.HasSuffix(name, "_test.go") || !catalogLiteralFileRe.MatchString(name) {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		rel, err := filepath.Rel(catalogLiteralRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		fset := token.NewFileSet()
		file := fset.AddFile(path, fset.Base(), len(src))
		var s scanner.Scanner
		s.Init(file, src, nil, 0)
		for {
			pos, tok, lit := s.Scan()
			if tok == token.EOF {
				break
			}
			if tok != token.FLOAT || !isNoisyCatalogFloatLiteral(lit) {
				continue
			}
			key := rel + "\t" + lit
			found[key]++
			locations[key] = append(locations[key], rel+":"+strconv.Itoa(fset.Position(pos).Line))
		}
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, scanned, 100, "the guard found too few catalog files under %s; it has stopped guarding anything", catalogLiteralRoot)

	var offenders, stale []string
	for _, key := range snapshotSortedKeys(found) {
		if found[key] > grandfatheredCatalogFloatLiterals[key] {
			_, lit, _ := strings.Cut(key, "\t")
			offender := strings.Join(locations[key], ", ") + ": " + lit
			if value, err := strconv.ParseFloat(strings.ReplaceAll(lit, "_", ""), 64); err == nil {
				if short, ok := shortCatalogDecimal(value); ok {
					offender += " (write " + short + ")"
				}
			}
			offenders = append(offenders, offender)
		}
	}
	for _, key := range snapshotSortedKeys(grandfatheredCatalogFloatLiterals) {
		if found[key] < grandfatheredCatalogFloatLiterals[key] {
			stale = append(stale, strings.ReplaceAll(key, "\t", " "))
		}
	}

	require.Emptyf(t, offenders,
		"catalog literals look like float64 printouts rather than published prices:\n  %s\n\n"+
			"Write the price exactly as the provider publishes it:\n"+
			"  - a short decimal: nativeRate(0.8), not nativeRate(0.7999999999999999); 10, not 1e+01;\n"+
			"  - a ratio of two published prices as their quotient: CompletionRatio: 1.1 / 0.28 (output / input);\n"+
			"  - a derived unit as an exact expression: UsdPerSecond: 0.0002 / 60 for 0.0002 USD per minute.\n"+
			"Untyped Go constant expressions are evaluated exactly, so these forms are at least as precise as the printout.",
		strings.Join(offenders, "\n  "))
	require.Emptyf(t, stale,
		"grandfathered noisy literals were removed or rewritten; delete their entries from "+
			"grandfatheredCatalogFloatLiterals so the list keeps shrinking: %v", stale)
}

// isNoisyCatalogFloatLiteral reports whether a Go float literal is a float64
// printout rather than a published decimal.
//
// Parameters:
//   - lit: the literal text as written in source.
//
// Returns: true when the mantissa has 14 or more fractional digits, or when the
// literal uses printed exponent notation (e+NN / e-NN) and its value has a short
// decimal form.
func isNoisyCatalogFloatLiteral(lit string) bool {
	if strings.HasPrefix(lit, "0x") || strings.HasPrefix(lit, "0X") {
		return false
	}
	mantissa, _, _ := strings.Cut(strings.ToLower(lit), "e")
	if catalogLiteralLongFractionRe.MatchString(mantissa) {
		return true
	}
	if !catalogLiteralPrintedExponentRe.MatchString(lit) {
		return false
	}
	value, err := strconv.ParseFloat(strings.ReplaceAll(lit, "_", ""), 64)
	if err != nil {
		return false
	}
	_, ok := shortCatalogDecimal(value)
	return ok
}

// shortCatalogDecimal finds the shortest plain decimal with at most 10
// significant digits that lies within catalogLiteralMaxULP of value.
//
// Parameters:
//   - value: the literal's float64 value.
//
// Returns: the decimal text and true, or false when no such decimal exists.
func shortCatalogDecimal(value float64) (string, bool) {
	for digits := 1; digits <= 10; digits++ {
		candidate, err := strconv.ParseFloat(strconv.FormatFloat(value, 'g', digits, 64), 64)
		if err != nil {
			continue
		}
		if catalogULPDistance(candidate, value) <= catalogLiteralMaxULP {
			return strconv.FormatFloat(candidate, 'f', -1, 64), true
		}
	}
	return "", false
}

// catalogULPDistance returns the number of representable float64 values
// between a and b.
//
// Parameters:
//   - a, b: finite float64 values.
//
// Returns: the absolute ULP distance.
func catalogULPDistance(a, b float64) uint64 {
	ordered := func(f float64) int64 {
		bits := int64(math.Float64bits(f))
		if bits < 0 {
			bits = math.MinInt64 - bits
		}
		return bits
	}
	x, y := ordered(a), ordered(b)
	if x > y {
		return uint64(x - y)
	}
	return uint64(y - x)
}

// TestIsNoisyCatalogFloatLiteral pins the classifier the catalog guard relies on.
func TestIsNoisyCatalogFloatLiteral(t *testing.T) {
	for lit, want := range map[string]bool{
		"0.7999999999999999":     true,
		"0.19999999999999998":    true,
		"3.9285714285714293":     true,
		"3.33333333333333":       true,
		"1e+01":                  true,
		"7.5e-06":                true,
		"3.3333333333333333e-06": true,
		"0.8":                    false,
		"100.0":                  false,
		"0.0000075":              false,
		"1e6":                    false,
		"2.5e-5":                 false,
		"0x1p-2":                 false,
	} {
		require.Equalf(t, want, isNoisyCatalogFloatLiteral(lit), "literal %s", lit)
	}
}
