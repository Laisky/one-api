# Mistral October continuation

Verified on 2026-10-12 against:

- https://docs.mistral.ai/models/mistral-large-4-0
- https://docs.mistral.ai/inference/pricing
- https://docs.mistral.ai/resources/changelogs
- https://docs.mistral.ai/studio/conversations/reasoning

Add `mistral-large-4` and the `mistral-large-4-0` name used in the official
reasoning guide. Both independently own their metadata and use the already
implemented Chat/Responses/Claude conversion and thinking-chunk handling.
Published context is 1M; no precise output ceiling, weight repository, or hosted
quantization is invented. Text/image input and text output are advertised, not
OCR endpoints, built-in Agents transport, or generated images.

## Billing boundary decision

The prior checkpoint deferred this model over the unspecified promotional
cutoff. This implementation makes the ambiguity explicit: standard list rates
USD 1.36 / 0.14 / 4.18 per million input / cached input / output tokens are the
base. The announced 50%-off, two-week promotion uses a bounded October 6–20
exclusive UTC overlay at USD 0.68 / 0.07 / 2.09. UTC calendar dates follow the
repository's existing date-only pricing convention. This is an implementation
convention, **not evidence of an exact upstream cutoff instant**. Operators
must reconcile the boundary against their invoices or override the schedule.
No launch discount can persist after that boundary. `mistral-large-latest`
remains unchanged because no inspected source establishes its remapping.

## GLM 5.3 reasoning bug

The current Mistral reasoning guide explicitly documents `low`, `high`, and
`max` for `zai-glm-5-3`. The previous catalogue left the effort list empty, which
made `reasoningRequest` discard valid caller effort values. Advertise only that
model's documented vocabulary and reasoning feature; retain GLM 5.2's previous
metadata and both models' Mistral-specific tariffs. No Z.AI price is copied.

Regression tests cover exact IDs, discovery, Chat and Responses wire controls,
caller non-mutation, seed/token normalization, retained tool payloads, independent
alias allocations, and the production time resolver at both nanosecond boundaries
and an equivalent UTC-4 instant. Validation receipts are tracked in PR #560.
No live-provider call is claimed.

Rollback: revert this provider commit. No schema or deployment change is required.
