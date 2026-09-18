package typesafe

import (
	"math"
	"math/big"
	"strconv"

	"github.com/Laisky/errors/v2"
)

// AdmissionInputTokens is a conservative allowance for the documented 64k
// request context. It is a reservation policy, not a local tokenizer result.
const AdmissionInputTokens = 64 * 1024

// InputQuota prices input tokens with decimal arithmetic and one upward rounding.
// Output counters are deliberately excluded: System One is input-token priced.
// The representability ceiling matches the gateway's safe monetary integer range.
func InputQuota(tokens int, inputRatio, groupRatio float64) (int64, error) {
	if tokens < 0 || inputRatio < 0 || groupRatio < 0 ||
		math.IsNaN(inputRatio) || math.IsNaN(groupRatio) ||
		math.IsInf(inputRatio, 0) || math.IsInf(groupRatio, 0) {
		return 0, errors.New("invalid TypeSafe token count or input pricing")
	}
	value := new(big.Rat).SetInt64(int64(tokens))
	for _, factor := range []float64{inputRatio, groupRatio} {
		ratio, ok := new(big.Rat).SetString(strconv.FormatFloat(factor, 'g', -1, 64))
		if !ok {
			return 0, errors.New("invalid decimal pricing factor")
		}
		value.Mul(value, ratio)
	}
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(value.Num(), value.Denom(), remainder)
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() || quotient.Int64() > 1<<52 {
		return 0, errors.New("TypeSafe charge exceeds the safe monetary range; reconciliation required")
	}
	return quotient.Int64(), nil
}

// IsAdmissionRejection recognizes only the provider's documented rejection codes.
// Other statuses, transport failures and interrupted bodies may represent paid work.
func IsAdmissionRejection(status int) bool {
	switch status {
	case 401, 422, 429, 529:
		return true
	default:
		return false
	}
}
