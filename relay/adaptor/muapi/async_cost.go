package muapi

import (
	"math/big"
	"strings"

	"github.com/Laisky/errors/v2"

	dbmodel "github.com/Laisky/one-api/model"
)

// muAPIMaxChargedCost merges validated body cost and the documented USD headers.
// An invalid header withholds success, but never erases another valid charge.
// Decimal comparison is exact and allocation is bounded before big.Rat parsing.
// Refund headers alone are deliberately NOT trusted as task-identity evidence.
func muAPIMaxChargedCost(bodyCost string, headers []string) (string, error) {
	charged := bodyCost
	var firstErr error
	if len(headers) > 16 {
		firstErr = errors.New("MuAPI cost header count exceeds limit")
		headers = headers[:16]
	}
	for _, raw := range headers {
		amount := strings.TrimSpace(raw)
		if _, err := dbmodel.AsyncUpstreamCostQuota("1", amount); err != nil {
			if firstErr == nil {
				firstErr = errors.New("MuAPI cost header is invalid")
			}
			continue
		}
		if charged == "" {
			charged = amount
			continue
		}
		previous, _ := new(big.Rat).SetString(charged)
		current, _ := new(big.Rat).SetString(amount)
		if current.Cmp(previous) > 0 {
			charged = amount
		}
	}
	return charged, firstErr
}
