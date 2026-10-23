package tooling

import (
	"math"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/common/deepseekcompat"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// ConservativeDeepSeekDefaultCalls is the minimum number of tool attempts charged
// for each positively priced hosted tool declaration. DeepSeek does not provide
// a documented bound on internally executed calls, so this is an explicit
// commercial estimate, not an upstream success receipt.
const ConservativeDeepSeekDefaultCalls int64 = 10

type deepSeekToolAllowance struct {
	minCalls     map[string]int64
	quotaPerCall map[string]int64
}

// RecordConservativeDeepSeekTools retains an admitted attempt's price and
// minimum billable counts. Repeated admission of the same tool uses the larger
// bound, not a second charge. For Claude, declaredUses carries max_uses.
func RecordConservativeDeepSeekTools(c *gin.Context, meta *metalib.Meta, quotes map[string]int64, declaredUses map[string]int64) {
	if c == nil || !deepseekcompat.UsesDeepSeekToolReceiptContract(meta) {
		return
	}
	var allowance *deepSeekToolAllowance
	if raw, ok := c.Get(ctxkey.DeepSeekConservativeToolAllowance); ok {
		allowance, _ = raw.(*deepSeekToolAllowance)
	}
	if allowance == nil {
		allowance = &deepSeekToolAllowance{
			minCalls:     make(map[string]int64),
			quotaPerCall: make(map[string]int64),
		}
	}
	for name, price := range quotes {
		if (name != BuiltinWebSearch && name != BuiltinWebFetch) || price <= 0 {
			continue
		}
		uses := max(ConservativeDeepSeekDefaultCalls, declaredUses[name])
		allowance.minCalls[name] = max(allowance.minCalls[name], uses)
		allowance.quotaPerCall[name] = max(allowance.quotaPerCall[name], price)
	}
	c.Set(ctxkey.DeepSeekConservativeToolAllowance, allowance)
}

func conservativeDeepSeekAllowance(c *gin.Context) *deepSeekToolAllowance {
	if c == nil {
		return nil
	}
	raw, _ := c.Get(ctxkey.DeepSeekConservativeToolAllowance)
	allowance, _ := raw.(*deepSeekToolAllowance)
	return allowance
}

// ConservativeDeepSeekToolQuota returns the complete funded tool floor, with
// saturating arithmetic. Zero means no positively priced tool was admitted.
func ConservativeDeepSeekToolQuota(c *gin.Context) int64 {
	allowance := conservativeDeepSeekAllowance(c)
	if allowance == nil {
		return 0
	}
	var total int64
	for name, count := range allowance.minCalls {
		price := allowance.quotaPerCall[name]
		cost := SaturatingToolAllowance(price, count)
		if cost >= math.MaxInt64-total {
			return math.MaxInt64
		}
		total += cost
	}
	return total
}

// conservativeDeepSeekCounts merges observed event counts with funded floors.
// Unsolicited Search/Fetch counters without an admitted positive tool cannot
// create a new DeepSeek charge. Unknown/non-Search tools are unchanged.
func conservativeDeepSeekCounts(c *gin.Context, counts map[string]int) (estimated bool) {
	allowance := conservativeDeepSeekAllowance(c)
	for _, name := range []string{BuiltinWebSearch, BuiltinWebFetch} {
		if allowance == nil || allowance.minCalls[name] == 0 {
			delete(counts, name)
			continue
		}
		minimum := allowance.minCalls[name]
		if minimum > int64(math.MaxInt) {
			minimum = int64(math.MaxInt)
		}
		if counts[name] < int(minimum) {
			counts[name] = int(minimum)
			estimated = true
		}
	}
	return estimated
}
