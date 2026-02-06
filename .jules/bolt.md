## 2026-02-06 - [Redundant model support checks in channel selection]

**Learning:** The channel cache in One API is already indexed by model name. Re-checking `SupportsModel` in a loop during every request is O(N) redundant work that involves string splitting and allocations. TRIPLE redundancy exists between the cache lookup, the cache selection functions, and the distributor middleware.
**Action:** Remove redundant `SupportsModel` checks in the cached path. Ensure `InitChannelCache` and `SupportsModel` use consistent trimming if needed, but for now, focus on removing the redundant checks in the hot path.
