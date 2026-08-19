# OpenRouter Upstream-Provider Integration

## Overview

This integration lets an operator list a one-api deployment as an upstream
provider on [OpenRouter](https://openrouter.ai/). OpenRouter polls a
public model-listing endpoint for the catalog and forwards user traffic to
the OpenAI-compatible chat-completions endpoint already exposed by one-api.

Supported in v1:

- Chat completions (streaming and non-streaming) over the existing
  OpenAI-compatible `POST /v1/chat/completions` route.
- Public model catalog at `GET /openrouter/v1/models` that conforms to the
  OpenRouter upstream-provider listing schema.

Out of scope for v1:

- Embeddings, rerank, text-to-speech, speech-to-text, image, and video
  endpoints. OpenRouter only consumes the chat-completions surface from
  this integration.

## Endpoints exposed

### `GET /openrouter/v1/models` (public)

Returns the full catalog of models the running one-api process knows
about, formatted to match the OpenRouter upstream-provider listing
schema. No authentication is required so OpenRouter can poll it.

Each item in `data` includes the following required fields: `id`,
`hugging_face_id`, `name`, `created`, `input_modalities`,
`output_modalities`, `quantization`, `context_length`,
`max_output_length`, `pricing`, `supported_sampling_parameters`,
`supported_features`.

Sample response (truncated):

```json
{"data":[{"id":"gpt-4o-mini","hugging_face_id":"","name":"gpt-4o-mini","created":1700000000,"input_modalities":["text"],"output_modalities":["text"],"quantization":"fp16","context_length":128000,"max_output_length":16384,"pricing":{"prompt":"0.00000015","completion":"0.0000006"},"supported_sampling_parameters":["temperature","top_p"],"supported_features":["tools"]}]}
```

Notes:

- Models discovered across all registered adaptors are merged. If the same
  model id is reported by more than one adaptor, the first one wins; later
  duplicates are dropped.
- Ordering is stable: entries are sorted by `id` so consumers see a
  deterministic catalog between polls.
- Route registration lives in
  [router/relay.go](../../router/relay.go) under the
  `openRouterRouter := router.Group("/openrouter/v1")` block.

### `POST /v1/chat/completions` (token-auth)

This is the standard OpenAI-compatible chat-completions endpoint that
already ships with one-api. OpenRouter forwards traffic here using the
operator-issued bearer token.

Streaming requirement: OpenRouter expects the final SSE chunk before
`[DONE]` to contain a `usage` object so it can record per-request token
counts. The OpenAI request flag
`"stream_options":{"include_usage":true}` is supported and produces this
chunk; pass it through verbatim in the OpenRouter provider config.

For full request/response semantics see the existing
[billing manual](billing.md) and
[channels manual](channels.md).

## Onboarding steps

1. Submit the OpenRouter onboarding form at
   <https://openrouter.ai/how-to-list> with:
   - List-models URL: `https://<your-host>/openrouter/v1/models`
   - Chat-completions URL: `https://<your-host>/v1/chat/completions`
   - Auth scheme: `Bearer`
   - API token: a dedicated key issued from the one-api admin UI for
     OpenRouter traffic. Do not reuse a key that is also issued to other
     consumers; you will want isolated quota and revocation.

2. Confirm with the OpenRouter operations team which model ids should be
   exposed. The default behaviour is that this integration advertises
   every model the system knows about. If OpenRouter wants only a
   subset, deploy a dedicated channel and token whose model list is
   restricted via the existing channel/token configuration; that
   filtering already works without code changes (see
   [channels.md](channels.md)).

3. Verify the deployment with the sample `curl` invocations below
   before notifying OpenRouter that the integration is live.

## Enriching model metadata (optional)

Defaults applied when an adaptor does not provide overrides:

- `quantization`: `fp16`
- `context_length`: `8192`
- `max_output_length`: `4096`
- `input_modalities`: `["text"]`
- `output_modalities`: `["text"]`
- `hugging_face_id`: `""`
- `supported_features`: `[]`
- `supported_sampling_parameters`: `[]`

To override these for specific models, set the optional fields on
`adaptor.ModelConfig` defined in
[relay/adaptor/interface.go](../../relay/adaptor/interface.go). All
fields are optional and have safe zero-value defaults, so partial
overrides are fine.

Example per-model override in a `constants.go`-style file:

```go
package myprovider

import "github.com/songquanpeng/one-api/relay/adaptor"

var ModelList = map[string]adaptor.ModelConfig{
    "my-vision-7b": {
        Ratio:                       0.0001,
        ContextLength:               131072,
        MaxOutputTokens:             8192,
        InputModalities:             []string{"text", "image"},
        OutputModalities:            []string{"text"},
        SupportedFeatures:           []string{"tools", "response_format"},
        SupportedSamplingParameters: []string{"temperature", "top_p", "top_k"},
        Quantization:                "bf16",
        HuggingFaceID:               "my-org/my-vision-7b",
        Description:                 "Vision-language model with tool use.",
    },
}
```

Pricing math: the internal `Ratio` is denominated in "quota per 1k
tokens", with the constant `MilliTokensUsd = 0.5`. The listing endpoint
converts to USD-per-token with `Ratio / 0.5 / 1e6`, which is what
OpenRouter expects in `pricing.prompt` and `pricing.completion`.

## Sample curl: list models

```bash
curl -sS https://your-host.example.com/openrouter/v1/models \
  -H 'Accept: application/json'
```

No `Authorization` header is required — this endpoint is public so
OpenRouter can poll it without a credential.

## Sample curl: chat completions

Non-streaming:

```bash
curl -sS https://your-host.example.com/v1/chat/completions \
  -H "Authorization: Bearer $ONEAPI_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      {"role": "user", "content": "Say hello in one word."}
    ]
  }'
```

Streaming with usage chunk (this is the variant OpenRouter uses):

```bash
curl -sS https://your-host.example.com/v1/chat/completions \
  -H "Authorization: Bearer $ONEAPI_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "gpt-4o-mini",
    "stream": true,
    "stream_options": {"include_usage": true},
    "messages": [
      {"role": "user", "content": "Say hello in one word."}
    ]
  }'
```

The final SSE event before `data: [DONE]` will carry a `usage` object;
OpenRouter relies on this to attribute prompt and completion token
counts to the request.

## Limitations

- `quantization` is reported as `fp16` for every model unless the
  adaptor explicitly overrides it on the `ModelConfig`.
- `hugging_face_id` is empty unless overridden; OpenRouter accepts an
  empty string but will not be able to cross-link the model card.
- `input_modalities` and `output_modalities` default to `["text"]`. Set
  them explicitly on the `ModelConfig` for multimodal models.
- The listing endpoint exposes every model registered in the running
  process. There is no built-in subset filter for the
  `/openrouter/v1/models` route. The supported workaround is to point
  OpenRouter at a deployment (or token/channel scope) whose model list
  is already restricted via the existing channel/token configuration.

## Troubleshooting

- **404 from `/openrouter/v1/models`** — the route group is not
  registered. Confirm the `openRouterRouter` block in
  [router/relay.go](../../router/relay.go) is present and the binary
  was rebuilt.
- **Empty `data` array** — no adaptors were initialized, so the merged
  catalog is empty. Check the startup logs for adaptor `Init` errors
  and verify that channels are configured. The listing endpoint walks
  all registered adaptors, so a missing adaptor presents as an empty
  catalog.
- **OpenRouter rejects the `pricing` field** — usually means an
  adaptor returned a model with a zero or unset `Ratio`. Make sure the
  adaptor's `GetDefaultModelPricing()` (defined per provider, see
  [relay/adaptor/interface.go](../../relay/adaptor/interface.go))
  returns a `Ratio` for every model id it advertises; otherwise the
  prompt/completion price strings will serialize as `"0"` and
  OpenRouter will reject the listing.
