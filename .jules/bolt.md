## 2026-02-06 - [Redundant model support checks in channel selection]

**Learning:** The channel cache in One API is already indexed by model name. Re-checking `SupportsModel` in a loop during every request is O(N) redundant work that involves string splitting and allocations. TRIPLE redundancy exists between the cache lookup, the cache selection functions, and the distributor middleware.
**Action:** Remove redundant `SupportsModel` checks in the cached path. Ensure `InitChannelCache` and `SupportsModel` use consistent trimming if needed, but for now, focus on removing the redundant checks in the hot path.

## 2026-02-06 - [Avoid Map Cloning in Hot Path]
**Learning:** Functions like `GetGlobalModelPricing()` that clone large maps and all their entries (thousands of models) introduce massive CPU and memory overhead when called on every request. This is a common performance anti-pattern in Go where safety (returning a copy) conflicts with performance in high-throughput paths.
**Action:** Always prefer targeted lookup functions (e.g., `GetGlobalModelConfig(name)`) over full collection retrieval in the request hot path. If such functions don't exist, create them or use a more efficient caching mechanism like `atomic.Pointer`.
