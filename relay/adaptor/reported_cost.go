package adaptor

// ReportedCostBillingPolicy is implemented by adaptors whose provider reports
// the authoritative charge of each request alongside its usage (for example
// OpenRouter's usage.cost). Settlement then never bills less than that
// reported charge times the group ratio, because the catalog tariff may
// describe only one of several providers the request can be routed to.
type ReportedCostBillingPolicy interface {
	// BillsAtLeastReportedCost reports whether settlement must floor the
	// computed charge at the provider-reported cost. It takes no parameters.
	BillsAtLeastReportedCost() bool
}

// BillsAtLeastReportedCost reports whether provider declares its reported
// per-request cost as a settlement floor. Parameter provider is the pricing
// adaptor and may be nil. It returns false for nil providers and providers
// that do not implement ReportedCostBillingPolicy.
func BillsAtLeastReportedCost(provider Adaptor) bool {
	if provider == nil {
		return false
	}
	policy, ok := provider.(ReportedCostBillingPolicy)
	return ok && policy.BillsAtLeastReportedCost()
}
