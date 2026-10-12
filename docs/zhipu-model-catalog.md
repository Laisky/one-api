# Zhipu / Z.AI model catalogue audit

Verified on **2026-10-12** for the standard, pay-as-you-go model APIs.

## Findings and changes

The current provider documentation and existing catalogue already contain
`glm-5.3`, `glm-5.3-flash`, and `glm-5.3-flashx`. This update corrects metadata,
not their existing prices or routing. It does not invent additional model IDs.

GLM-5.3 is text-only. Flash and FlashX accept text, images, video and files.
All three have a 1,000,000-token context and a 131,072-token maximum output.
All three require thinking and accept `low`, `high`, and `max` reasoning effort;
the default is `max`. Flash was missing the explicit effort list and default.

For the audited family, advertise the documented sampling controls
`temperature`, `top_p`, `max_tokens`, `stop`, and `reasoning_effort`. Do not
advertise unverified `top_k`, penalty, seed or logit-bias controls. Keep function
calling, JSON mode, web search and reasoning. The structured-output guide
specifies `json_object` and client-side schema validation, not provider-enforced
`json_schema`; remove that stronger capability claim from GLM-5.3 and Flash.
FlashX already had the restricted capability set and remains a regression control.

Link GLM-5.3 and Flash to their verified `zai-org` Hugging Face repositories.
Do not infer a separate FlashX weights release or API-serving quantization.
Z.AI continues to clone BigModel metadata while replacing pricing independently.
No new initializer, override layer, catalogue generator or permanent CI is added.

## Prices retained

All figures are per million tokens. Currency is provider-specific; Z.AI prices
must never be derived by converting BigModel's CNY prices.

| API / model | Input | Cached input | Output |
| --- | ---: | ---: | ---: |
| BigModel / GLM-5.3 | CNY 8 | CNY 2 | CNY 28 |
| BigModel / GLM-5.3-Flash | CNY 0.8 | CNY 0.23 | CNY 2.8 |
| BigModel / GLM-5.3-FlashX | CNY 2 | CNY 0.57 | CNY 7 |
| Z.AI / GLM-5.3 | USD 1.40 | USD 0.26 | USD 4.40 |
| Z.AI / GLM-5.3-Flash | USD 0.15 | USD 0.03 | USD 0.50 |
| Z.AI / GLM-5.3-FlashX | USD 0.37 | USD 0.075 | USD 1.25 |

The existing 50% Flash launch windows remain available for historical pricing.
They expire at 2026-09-09 16:00:00 UTC (2026-09-10 00:00:00 UTC+8).
Current requests use the full list prices above. FlashX has no launch window.
Existing older-model length tiers, free entries, audio/image/video tariffs,
legacy IDs and operator overrides are unchanged.

## API scope and compatibility

Standard API support is separate from Coding Plan availability. In particular,
FlashX's absence from Coding Plan is not a reason to remove its model-API entry.
Do not add aggregator-only aliases or use another host's catalogue as evidence
of BigModel/Z.AI API support. This audit does not establish a first-party
`glm-5.3-prime` endpoint or tariff, so no such entry is added.

This is a model-information change, not a request-normalization change. It does
not silently rewrite `thinking.type: disabled` to a paid reasoning request.
The provider documents that disabled thinking fails for this family; clients
should explicitly choose enabled thinking and a supported effort. No endpoint,
Chat/Responses/Messages conversion, authentication, schema or deployment changes
are included. Rollback is a normal revert of this PR; no data migration is needed.

## Regression coverage

`relay/adaptor/zhipu/catalog_metadata_test.go` uses both public adaptors and the
real pricing resolver. It covers discovery/serialization, exact effort values,
capability exclusions, limits, modalities, weights IDs, independent backing
slices, six provider-specific tariffs, exact promotion-expiry boundaries, and
the older BigModel 32K tier versus Z.AI flat-price distinction.

Run `go test -race -count=1 ./relay/adaptor/zhipu ./relay/adaptor/zai ./relay/pricing`
and `make test-quick`. These deterministic tests do not call paid provider APIs;
they are not a live-provider smoke test or a full-repository qualification.

## Primary sources

- [BigModel model overview](https://docs.bigmodel.cn/cn/guide/start/model-overview)
- [BigModel pricing](https://docs.bigmodel.cn/cn/guide/start/pricing)
- [Z.AI pricing](https://docs.z.ai/guides/overview/pricing)
- [GLM-5.3 guide](https://docs.z.ai/guides/llm/glm-5.3)
- [GLM-5.3-Flash / FlashX guide](https://docs.z.ai/guides/vlm/glm-5.3-flash)
- [Standard Chat Completion API](https://docs.z.ai/api-reference/llm/chat-completion)
- [Structured output / JSON mode](https://docs.z.ai/guides/capabilities/struct-output)
- [GLM-5.3 weights](https://huggingface.co/zai-org/GLM-5.3)
- [GLM-5.3-Flash weights](https://huggingface.co/zai-org/GLM-5.3-Flash)
