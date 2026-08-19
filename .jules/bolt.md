## 2026-02-06 - [Redundant model support checks in channel selection]

**Learning:** The channel cache in One API is already indexed by model name. Re-checking `SupportsModel` in a loop during every request is O(N) redundant work that involves string splitting and allocations. TRIPLE redundancy exists between the cache lookup, the cache selection functions, and the distributor middleware.
**Action:** Remove redundant `SupportsModel` checks in the cached path. Ensure `InitChannelCache` and `SupportsModel` use consistent trimming if needed, but for now, focus on removing the redundant checks in the hot path.

## 2026-02-06 - [Avoid Map Cloning in Hot Path]

**Learning:** Functions like `GetGlobalModelPricing()` that clone large maps and all their entries (thousands of models) introduce massive CPU and memory overhead when called on every request. This is a common performance anti-pattern in Go where safety (returning a copy) conflicts with performance in high-throughput paths.
**Action:** Always prefer targeted lookup functions (e.g., `GetGlobalModelConfig(name)`) over full collection retrieval in the request hot path. If such functions don't exist, create them or use a more efficient caching mechanism like `atomic.Pointer`.

## 2026-02-09 - [Avoid Redundant Full-Config Cloning in Global Pricing]

**Learning:** Calling GetGlobalModelConfig() on every request to fetch a single float (ratio) is extremely wasteful as it performs a deep clone of the entire ModelConfig struct, including multiple maps (ImagePricingConfig) and slices (Tiers). This pattern introduces significant allocation overhead and GC pressure in the hot path.
**Action:** Use specialized, lightweight getters (e.g., GetGlobalModelRatio) that return (value, bool) or only clone the necessary sub-struct. This achieved a ~13.6x speedup (678ns -> 50ns) in global pricing resolution benchmarks.

## 2026-02-10 - [Remove Redundant Map Lookups in Three-Layer Pricing]

**Learning:** Redundant map lookups in the hot path of pricing resolution ( and ) introduced unnecessary overhead (~21ns per op). This was caused by calling (which performs a lookup) and then calling again to verify if the model was specifically defined in the adapter.
**Action:** Use directly to perform a single lookup and retrieve the ratio. This simple change provided a ~44% speedup in benchmarks for these core functions.

## 2026-02-10 - [Remove Redundant Map Lookups in Three-Layer Pricing]

**Learning:** Redundant map lookups in the hot path of pricing resolution (`GetModelRatioWithThreeLayers` and `GetCompletionRatioWithThreeLayers`) introduced unnecessary overhead (~21ns per op). This was caused by calling `adaptor.GetModelRatio(modelName)` (which performs a lookup) and then calling `adaptor.GetDefaultModelPricing()[modelName]` again to verify if the model was specifically defined in the adapter.
**Action:** Use `adaptor.GetDefaultModelPricing()` directly to perform a single lookup and retrieve the ratio. This simple change provided a ~44% speedup in benchmarks for these core functions.

## 2026-02-20 - [Redundant Clones and Over-eager Conversion in Media Pricing]

**Learning:** Resolving Audio/Image pricing from channel overrides previously converted the entire model configuration and then cloned the result again. This resulted in redundant heap allocations and unnecessary CPU cycles for converting unused media types (e.g., converting Video/Image when only Audio was needed).
**Action:** Use targeted conversion functions (e.g., ) and remove redundant clones when the object is already a freshly created local copy. This streamlines the pricing resolution path for media-heavy requests.

## 2026-02-20 - [Redundant Clones and Over-eager Conversion in Media Pricing]

**Learning:** Resolving Audio/Image pricing from channel overrides previously converted the entire model configuration and then cloned the result again. This resulted in redundant heap allocations and unnecessary CPU cycles for converting unused media types (e.g., converting Video/Image when only Audio was needed).
**Action:** Use targeted conversion functions (e.g., `convertLocalAudio`) and remove redundant clones when the object is already a freshly created local copy. This streamlines the pricing resolution path for media-heavy requests.

## 2026-03-05 - [Redundant pricing lookups and deep-cloning in quota calculation]

**Learning:** The `quota.Compute` function, a critical hot path for every request, was performing multiple redundant lookups (`GetCompletionRatioWithThreeLayers`, `ResolveEffectivePricing`, `ResolveModelConfig`) that each performed similar map lookups and expensive deep-cloning of `ModelConfig` objects. These objects contain large nested structures for media pricing (Image, Audio, Video) that are entirely unused during standard token-based quota calculation.

**Action:** Consolidate pricing resolution into a single `ResolveModelConfigRatioOnly` call that performs a shallow clone of the base struct and a targeted clone of the `Tiers` slice, while omitting media metadata. This reduced the `BenchmarkCompute` execution time by ~19% (2313ns -> 1874ns). Always use "RatioOnly" or targeted lookup functions when full configuration metadata is not required in high-throughput paths.
