# Baichuan M-series catalogue audit — 2026-10-12

Sources:
- https://platform.baichuan-ai.com/prices
- https://platform.baichuan-ai.com/docs/medical

The medical API specifies max_tokens from 1 through 32000 for Baichuan M2,
M2-Plus, M3 and M3-Plus. Replace the inherited 2048-token metadata with 32000
and restrict advertised sampling controls to the documented temperature/top_p/
top_k/max_tokens set. No runtime hard limit, model ID or tariff is changed.
The official price table matches existing native CNY rates (per million):
M2 2/20; M2-Plus 10/30; M3 10/30; M3-Plus 5/9. Embedding and historical daypart
prices remain unchanged. Non-medical models do not inherit this API's limits.

Medical search can be triggered automatically by Plus models and costs CNY0.03
per call; that usage-dependent charge is not a token-price multiplier and is not
implemented by this metadata correction. File-purpose/medical-evidence controls
and native thinking budgets also require independent protocol qualification.
The common compatible request conversion is not certified as a full medical
API implementation by a catalogue-only test. No account or live call is made.

Public-catalogue regressions pin new limits, exact tariffs and neighboring model
metadata. Publishing requires pre-fix assertion failures and full affected
package race tests. Revert this provider commit to roll back metadata only.
