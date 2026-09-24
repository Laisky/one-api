# Maintaining model defaults

Model defaults are production Go code. Research the provider's documented API,
then edit the appropriate models_*.go file and the affected behavior tests. There
is no generation command, source lock, embedded catalog, or runtime patch loader.

Each model ID has one complete definition within its provider. Family constructors
are assembled with JoinModelCatalogs, which rejects duplicates instead of applying
overrides. Provider-specific differences, including Alibaba versus AliBailian,
remain explicit in separate definitions. Constructors return independently owned
maps and nested pricing metadata; common configuration is not assumed from a
shared source URL.

Prices and provenance
---------------------
The nativeRate helper expresses a native-currency per-million input-unit price in
the project's existing billing units. Output/input multipliers, free-cache
sentinels, specialized audio/image/embedding/per-call units, input thresholds,
timezone/date windows and unknown fields retain their existing semantics. A rare
literal already in quota units preserves a pre-existing rounding result exactly.
Do not replace a provider's tariff with the model creator's price or infer that an
unpublished price is zero. Do not turn catalog metadata into account restrictions.

Source URLs remain next to the provider definitions. The September 2026 research
ledger records provider limitations and exclusions. Full historical audit reports
and source locks remain available in Git history, not as production dependencies.

Verification
------------
Run the affected provider tests and the cross-provider pricing, request/response,
listing, ownership and isolation regressions. Changes to effective prices are
intentional code changes, not side effects of an imported data file. Run the
normal repository vet/race/CI checks before merging.
