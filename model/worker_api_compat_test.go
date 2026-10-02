package model

// Compile-time compatibility checks for exported periodic-worker entry points.
// Context-aware variants are additive; embedders using the historical
// frequency-only functions must continue to compile.
var (
	_ func(int) = SyncOptions
	_ func(int) = SyncChannelCache
)
