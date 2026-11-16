# Maintaining model defaults

Model defaults are production Go code. Start at the provider's `models.go` when
checking or updating its catalog. Small catalogs live entirely in that file;
larger catalogs use it as the assembly entry point for `models_<family>.go`
files. Split by a stable model family or capability when needed to keep each
handwritten Go file within 800 lines. Protocol constants and request/response
types belong outside the catalog files.

Update an existing model in its family, and put a new model alongside its
siblings. Do not create files or registration functions named after a research
date, month, update batch, or arbitrary numbered continuation. For example, an
Alibaba DeepSeek addition belongs in `models_deepseek.go`, regardless of the
month it was researched. Keep research dates in source comments and audit
documents. Catalog assembly belongs in `models.go`, so locating the effective
definitions does not require following a succession of dated `init` patches.

Research the provider's documented API, then edit the appropriate definition
and affected behavior tests. There is no generation command, source lock,
embedded catalog, or runtime patch loader.

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

Source URLs remain next to the provider definitions. The
[November 16, 2026 catalogue review](../../docs/adaptor-model-refresh-20261116.md)
records every adaptor directory's disposition, source-backed updates, provider
checkpoints, qualification evidence, and explicit exclusions. Directory coverage
is not live-provider certification: account-specific tariffs, historical IDs,
promotional boundaries, and unsupported native protocols require separate care.
Historical audit reports and source locks remain available in Git history, not
as production dependencies.

Verification
------------
Run the affected provider tests and the cross-provider pricing, request/response,
listing, ownership and isolation regressions. Changes to effective prices are
intentional code changes, not side effects of an imported data file. Run the
normal checks under the [repository testing policy](../../docs/ci-testing.md).
For a layout-only change, compare the compiled catalogs before and after using
`TestCompiledAdaptorCatalogSnapshot`; prices, metadata, discovery lists and
provider distinctions must remain identical.
