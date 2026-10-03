# Model parameter compatibility

Verified against provider documentation on 2026-09-24.

## Problem

A request routed to `gpt-6-luna` can receive an upstream Responses 400 when an SDK supplies `temperature` and reasoning is active. Luna defaults to `medium`, so omitting the reasoning configuration does not make sampling valid.

Native Responses forwards raw JSON through `normalizeResponseAPIRawBody`, rather than running the Chat adaptor transformations. Cleaning only a typed Chat request therefore misses the native path. Controlled passthrough can also restore fields removed earlier. Compatibility must be enforced on the final mapped wire object after extension merging.

## Verified contracts and actions

| Provider / model | Documented behavior | Action |
| --- | --- | --- |
| OpenAI GPT-6 Luna and Sol | Default effort is `medium`; `none` is supported. Active reasoning rejects `temperature`, `top_p`, `top_logprobs`, Chat `logprobs`, and the Responses logprobs include selector. | Use the final upstream model's catalog default and effective effort. Strip rejected controls when reasoning is active; retain supported controls, including explicit zero values, at `none`. |
| OpenAI GPT-6 Astra and cataloged reasoning-only models | Astra has no `none` mode; the existing catalog describes reasoning defaults and allowed efforts. | Apply the reasoning sampling contract without inventing a non-reasoning mode. Ordinary invalid-effort validation is not replaced with an arbitrary fallback. |
| Groq Chat Completions | `logprobs`, `logit_bias`, `top_logprobs`, and `messages[].name` are rejected. `n` is accepted only at 1. | Remove the unsupported fields at Groq dispatch, including converted Claude requests and Responses-to-Chat fallback. Also enforce the adaptor's existing `top_k` omission after merging. Keep tool function names, tool-call IDs, output limits, supported sampling, and `n` unchanged; a request for multiple outputs must not silently become one output. |
| Anthropic Claude Mythos Preview | The sampling deprecation also applies to Mythos Preview, not only the newer Opus/Claude families already handled in `compat.go`. | Extend sampling cleanup to Mythos Preview and its suffixed IDs. Keep this decision separate from thinking-mode conversion, so sampling cleanup does not enable thinking. |
| DeepSeek Flash | Thinking mode ignores `temperature`; `top_p` remains meaningful in thinking mode with an effective 0.95–1.0 range. Responses also supports `top_logprobs`. | Do not apply the OpenAI reasoning blacklist to DeepSeek. Add isolation coverage rather than removing supported parameters. |
| Gemini 3.x | Google discourages changing default sampling settings but documents that they can be modified. | Treat a recommendation differently from an unsupported-field error. Do not add a blanket sampling blacklist for Gemini. |

Sources:

- OpenAI migration / parameter guidance: https://developers.openai.com/api/docs/guides/latest-model
- Luna: https://developers.openai.com/api/docs/models/gpt-6-luna
- Sol: https://developers.openai.com/api/docs/models/gpt-6-sol
- Astra: https://developers.openai.com/api/docs/models/gpt-6-astra
- Groq compatibility: https://console.groq.com/docs/openai
- Anthropic sampling deprecation: https://platform.claude.com/docs/en/about-claude/model-deprecations
- DeepSeek thinking: https://api-docs.deepseek.com/guides/thinking_mode/
- DeepSeek Responses compatibility: https://api-docs.deepseek.com/guides/responses_api/
- Gemini sampling guidance: https://ai.google.dev/gemini-api/docs/troubleshooting

This is a focused audit of sampling and related optional fields, not a certification of every parameter on every provider or model.

## Implementation boundaries

`openai.NormalizeModelRequestParameters` resolves a contract only for cataloged models and valid dated snapshots of a known catalog family. Arbitrary names beginning with `o` or `gpt` do not establish a contract. Unknown models and other providers retain their parameters.

The shared controlled merge applies the policy after model mapping and extension merging. The typed Responses serializer provides the same sampling guard for typed fallbacks and converted requests. Chat normalization checks the effective effort before discarding supported sampling. No additional inference attempt is introduced.

Raw JSON values remain `json.RawMessage`, preventing large integer values from being converted through `float64`. Include filtering retains unrelated entries such as `reasoning.encrypted_content`, which callers may need for continuation. Serialization clones the include slice before removing entries, so shared request state is not mutated.

Groq applies its contract at its own dispatch boundary, not globally by model-name guessing. Multipart/audio modes retain their original readers. Native Responses bodies without a Chat `messages` field do not receive the Chat-specific rule.

Do not infer a permanent blacklist from arbitrary upstream 400 text. Do not automatically remove conversation selectors, prompts, tools, schemas, safety settings, output limits, or billing-related controls to turn an error into a success. Such changes can alter the meaning, privacy, or cost of a request. Invalid values and unsupported capabilities outside the documented optional-field rules still use normal error handling.

## Regression coverage

The added tests cover native Responses on OpenAI, Azure, and OpenAI-compatible channels; mapped tenant aliases; default and explicit reasoning; supported `none`; zero-valued controls; encrypted-reasoning include preservation; exact large integers; final Chat passthrough; typed serialization without shared-slice mutation; dated model IDs; and isolation from DeepSeek, Gemini, Grok, GPT-OSS, Qwen, and unknown custom models.

Groq tests cover rejected fields, message names versus tool function names, tool-call correlation, malformed/non-Chat data, dispatch modes, read failures, and idempotence. Anthropic tests cover Mythos Preview sampling without changing the requested thinking mode, nil pointers, repeat normalization, and preservation of older/custom model controls.

No test calls a paid inference API.

### Executed in the editing sandbox

The following run against the actual standard-library-only production helper files and their committed tests, not a reimplementation:

```sh
cd relay/adaptor/openai
go test -race -count=20 -cover request_parameter_policy.go request_parameter_policy_test.go
go vet request_parameter_policy.go request_parameter_policy_test.go
# PASS; isolated helper statement coverage: 100.0%

cd ../groq
go test -race -count=20 -cover request_parameter_policy.go request_parameter_policy_test.go
go vet request_parameter_policy.go request_parameter_policy_test.go
# PASS; isolated helper statement coverage: 91.7%
```

These percentages are not package or repository coverage. The repository-level regression tests, build, and vet were not executed in that sandbox: it has Go 1.23.2 while the repository requires Go 1.27.1, and cannot resolve the external hosts needed to obtain the repository/toolchain/dependencies. Full CI remains required before merge.

### Repository validation

Run with the repository's declared Go toolchain:

```sh
go test ./relay/adaptor/openai ./relay/adaptor/groq ./relay/adaptor/anthropic ./relay/controller
go test -race ./relay/adaptor/openai ./relay/adaptor/groq ./relay/adaptor/anthropic ./relay/controller
go test ./...
go vet ./...
go build -o one-api .
```

Only report a repository-wide pass after those checks actually finish successfully. A queued CI run or successful isolated helper test is not an end-to-end validation result.
