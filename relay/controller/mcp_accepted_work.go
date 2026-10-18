package controller

import relaymodel "github.com/Laisky/one-api/relay/model"

// retainMCPAcceptedUsage preserves valid evidence for an accepted but uncertain round.
// Parameters: prior is the last verified receipt and reason labels missing evidence.
// Returns: an independent usage value retaining established counters and quote floor;
// the existing estimate policy retains actual reservations without inventing a charge.
func retainMCPAcceptedUsage(prior *relaymodel.Usage, reason string) *relaymodel.Usage {
	retained := &relaymodel.Usage{}
	if prior != nil {
		*retained = *prior
		if prior.PromptTokensDetails != nil {
			details := *prior.PromptTokensDetails
			retained.PromptTokensDetails = &details
		}
	}
	if retained.BillingEstimateReason == "" {
		retained.BillingEstimateReason = reason
	}
	return retained
}
