package otelbridge

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

// jsonLogNumber maps precisely representable int64 tokens to integers. Decimal
// tokens use a double only when its JSON round trip has the same numeric value;
// otherwise their original spelling is retained as text. Comparison work is
// bounded independently of the accepted token size: large tokens stay exact text.
func jsonLogNumber(raw string) attribute.Value {
	if !strings.ContainsAny(raw, ".eE") && raw != "-0" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return attribute.Int64Value(n)
		}
		return attribute.StringValue(raw)
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return attribute.StringValue(raw)
	}
	encoded, err := json.Marshal(n)
	if err != nil || !sameJSONNumber(raw, string(encoded)) {
		return attribute.StringValue(raw)
	}
	return attribute.Float64Value(n)
}

// sameJSONNumber never expands an unbounded exponent into a big integer.
func sameJSONNumber(left, right string) bool {
	for _, value := range []string{left, right} {
		if len(value) > 4096 {
			return false
		}
		if i := strings.IndexAny(value, "eE"); i >= 0 {
			exponent, err := strconv.Atoi(value[i+1:])
			if err != nil || exponent < -4096 || exponent > 4096 {
				return false
			}
		}
	}
	a, ok := new(big.Rat).SetString(left)
	if !ok {
		return false
	}
	b, ok := new(big.Rat).SetString(right)
	return ok && a.Cmp(b) == 0
}

// validJSONSurrogates rejects escapes that encoding/json would silently repair.
// Its input is already JSON syntax checked by json.Marshal, so only the scalar
// Unicode boundary remains; escaped backslashes are not surrogate escapes.
func validJSONSurrogates(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		value, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value < 0xd800 || value > 0xdbff {
			continue
		}
		if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}
