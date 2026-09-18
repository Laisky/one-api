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

// IsAdmissionRejection reports whether a status proves the provider rejected the
// request before performing any billable evaluation.
//
// System One is a single-shot, non-streaming API that reports usage only on a
// successful evaluation. Live probes on 2026-09-18 confirmed that every client
// error is raised by the request-admission layer and carries no usage receipt:
//
//	400 {"detail":{"error_type":"max_tokens_exceeded"}}                  context budget
//	400 {"detail":{"error_type":"api_usage_error","message":"Unknown model: ..."}}
//	400 {"detail":"Noul question must have criteria or instructions: q"} primitive rules
//	400 {"detail":"Too many score levels. Must have at most 10 levels."} documented caps
//	401 {"detail":{"error_type":"authentication_error", ...}}            invalid key
//	403 {"detail":{"error_type":"authentication_error", ...}}            absent key
//	404 / 405                                                            wrong base URL or method
//	422 {"detail":[{"type":"missing","loc":["body","state"], ...}]}      schema validation
//
// The provider's published error table lists only 401/422/429/529, so restricting
// the refund to that subset silently billed the full admission reservation for
// the most common real failures (an oversized state, a mistyped model, a channel
// pointed at the wrong base URL). Every 4xx is therefore treated as unpaid, and
// 529 "Overloaded" is unpaid by definition. Ambiguous 5xx statuses and transport
// failures keep the reservation and are reported as estimates instead.
func IsAdmissionRejection(status int) bool {
	return (status >= 400 && status < 500) || status == StatusOverloaded
}

// StatusOverloaded is TypeSafe's documented "temporarily overloaded" status. It
// is outside net/http's registered codes, so it is named here rather than inline.
const StatusOverloaded = 529
