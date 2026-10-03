package dto

// EnabledAbility represents channel metadata exposed to API consumers for an ability lookup.
//
// One row per (model, channel) pair: the same model name served by several
// channels yields several abilities. Priority mirrors the parent channel's
// priority as of the last ability rebuild and follows the router's convention
// that HIGHER wins, so callers that must collapse the rows to a single channel
// (model listings, for example) can rank them the same way routing does --
// highest priority first, lowest channel id as the tie-break.
type EnabledAbility struct {
	Model       string `json:"model" gorm:"model"`
	ChannelType int    `json:"channel_type" gorm:"channel_type"`
	ChannelId   int    `json:"channel_id" gorm:"channel_id"`
	Priority    int64  `json:"priority" gorm:"priority"`
}

// Beats reports whether a ranks ahead of b as the channel that should represent
// a shared model name: higher priority first, then the lower channel id.
//
// This mirrors model/ability.go's routing preference (which keeps only the
// MAX(priority) tier) so a listing attributes a model to a channel that would
// plausibly serve it, and the lowest-id tie-break makes the choice a pure
// function of the rows rather than of database row order or map iteration.
//
// It is NOT time-invariant: a channel whose ability is transiently suspended
// (relay_error.go suspends on 429/5xx/auth for 30-60s) drops out of the input,
// so the reported owner can move to the runner-up for the duration. That
// matches what routing would do, which is the point -- but it means callers must
// not treat owned_by as a stable identifier.
func (a EnabledAbility) Beats(b EnabledAbility) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	return a.ChannelId < b.ChannelId
}
