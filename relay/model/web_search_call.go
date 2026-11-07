package model

// Web search call statuses carried by WebSearchCall. They use the Responses
// web_search_call status vocabulary so renderers can copy them verbatim.
const (
	// WebSearchCallInProgress marks a search the provider has started but whose
	// result block has not been observed yet.
	WebSearchCallInProgress = "in_progress"
	// WebSearchCallCompleted marks a search whose result block arrived.
	WebSearchCallCompleted = "completed"
	// WebSearchCallFailed marks a search that returned an error result or never
	// produced a result before the upstream message ended.
	WebSearchCallFailed = "failed"
)

// WebSearchCall describes one provider-executed web search observed on a
// converted upstream response. ID is the provider invocation identifier, Query
// is the searched text when known, and Status is one of the WebSearchCall*
// constants. It is response-only metadata and never billing evidence: billing
// uses the provider usage receipt.
type WebSearchCall struct {
	ID     string `json:"id"`
	Query  string `json:"query,omitempty"`
	Status string `json:"status,omitempty"`
}
