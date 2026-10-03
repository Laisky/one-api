package metrics

// Response-state metric label vocabulary (proposal ST-014 / OBS01-OBS05).
//
// IMPORTANT: these are the ONLY values that may be passed as the (category,
// outcome) labels of RecordResponseStateEvent. Every value is a compile-time
// constant so the metric stays low-cardinality; never pass a gateway response or
// conversation id, a prompt, a model name, or an error message as a label.
const (
	// Categories.
	StateCategoryPath        = "path"        // how a turn's effective context was resolved
	StateCategoryPortability = "portability" // per-item lowering decision on a fallback route
	StateCategoryCommit      = "commit"      // response-node commit outcome
	StateCategoryAffinity    = "affinity"    // pre-routing provider-affinity decision
	StateCategoryMiss        = "miss"        // state lookup miss classification

	// Path outcomes.
	StateOutcomeHydrated     = "hydrated"     // prior chain replayed into the turn
	StateOutcomeConversation = "conversation" // conversation snapshot replayed
	StateOutcomeStateless    = "stateless"    // no gateway state applied

	// Portability outcomes.
	StateOutcomePortable       = "portable"        // item carried as-is
	StateOutcomeSidecarDropped = "sidecar_dropped" // reasoning/thinking degraded to display-only
	StateOutcomeNotPortable    = "not_portable"    // hosted-tool/unknown item failed closed

	// Commit outcomes.
	StateOutcomeCommitted    = "committed"
	StateOutcomeCommitFailed = "commit_failed"
	StateOutcomeNoStore      = "no_store" // store=false: no shared record written

	// Affinity outcomes.
	StateOutcomePinned   = "pinned"   // bound channel preferred
	StateOutcomeUnpinned = "unpinned" // binding ineligible; normal selection

	// Miss outcomes (kept internal; never widen to raw causes).
	StateOutcomeNotFound   = "not_found"
	StateOutcomeStoreError = "store_error"
)

// RecordStateEvent is a convenience wrapper over Recorder() that tolerates a
// nil recorder (Recorder() is always set, but guarding keeps call sites and
// tests robust). Callers pass only the compile-time constants above.
func RecordStateEvent(category, outcome string) {
	Recorder().RecordResponseStateEvent(category, outcome)
}
