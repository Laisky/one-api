package state

// Limits bounds the size and shape of hydrated state so an untrusted client
// cannot force unbounded allocation or an oversized upstream transcript
// (Section 8.8, rows L01-L05) and so a single user cannot grow gateway state
// without bound (rows L06-L10). Every limit is configurable; a non-positive
// value disables that particular bound, which is how the feature stays inert
// when disabled (row L05).
type Limits struct {
	// MaxChainDepth bounds how many parent nodes a previous_response_id chain may
	// traverse during hydration.
	MaxChainDepth int
	// MaxItemCount bounds the number of hydrated items in one effective turn.
	MaxItemCount int
	// MaxRecordBytes bounds the encoded size of a single stored record.
	MaxRecordBytes int
	// MaxHydratedBytes bounds the total decoded transcript bytes for one turn.
	MaxHydratedBytes int
	// MaxHydratedTokens bounds the estimated hydrated prompt tokens for one turn.
	MaxHydratedTokens int

	// MaxResponsesPerUser bounds how many stored response records one user may
	// retain. On overflow the user's OLDEST records are pruned first (TTL+LRU);
	// an evicted parent then degrades to the standard previous_response_not_found
	// contract (row L06). Zero disables the cap.
	MaxResponsesPerUser int
	// MaxConversationsPerUser bounds how many active conversations one user may
	// hold. Creating beyond the cap fails with state_limit_exceeded; existing
	// conversations are unaffected and are never silently evicted (row L07).
	// Zero disables the cap.
	MaxConversationsPerUser int
}

// DefaultLimits returns conservative production defaults. They are intentionally
// generous enough for normal multi-turn tool workflows yet finite so a malicious
// or runaway chain is rejected before an upstream call.
func DefaultLimits() Limits {
	return Limits{
		MaxChainDepth:           64,
		MaxItemCount:            2048,
		MaxRecordBytes:          8 << 20,   // 8 MiB per record
		MaxHydratedBytes:        32 << 20,  // 32 MiB per hydrated turn
		MaxHydratedTokens:       1_000_000, // gated further by the target model context window
		MaxResponsesPerUser:     20000,
		MaxConversationsPerUser: 2000,
	}
}

// ChainDepthExceeded reports whether depth is beyond the configured maximum.
func (l Limits) ChainDepthExceeded(depth int) bool {
	return l.MaxChainDepth > 0 && depth > l.MaxChainDepth
}

// ItemCountExceeded reports whether count is beyond the configured maximum.
func (l Limits) ItemCountExceeded(count int) bool {
	return l.MaxItemCount > 0 && count > l.MaxItemCount
}

// RecordBytesExceeded reports whether size is beyond the configured maximum.
func (l Limits) RecordBytesExceeded(size int) bool {
	return l.MaxRecordBytes > 0 && size > l.MaxRecordBytes
}

// HydratedBytesExceeded reports whether size is beyond the configured maximum.
func (l Limits) HydratedBytesExceeded(size int) bool {
	return l.MaxHydratedBytes > 0 && size > l.MaxHydratedBytes
}

// HydratedTokensExceeded reports whether tokens is beyond the configured maximum.
func (l Limits) HydratedTokensExceeded(tokens int) bool {
	return l.MaxHydratedTokens > 0 && tokens > l.MaxHydratedTokens
}
