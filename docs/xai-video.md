# Grok video generation through one-api

Reviewed against xAI's [video generation guide](https://docs.x.ai/developers/model-capabilities/video/generation) and [Imagine Video 1.5 model page](https://docs.x.ai/developers/models/grok-imagine-video-1.5) on 2026-09-22.

## Configure a channel

Use the xAI channel type, an xAI API key, and the provider base URL `https://api.x.ai` (a base ending in `/v1` also works). Enable `grok-imagine-video-1.5` in the channel's models and allow the caller's group/token to use it. Upstream account access is an operator configuration concern, not a catalog filter.

This integration implements native JSON video generation and polling. It preserves image references, pinned frames, reference images, preset voice references, and other native generation options. The provider still validates model-specific combinations. It does not implement video editing, extension, listing, deletion, or the OpenAI `/content` download operation.

## Create and poll

```sh
curl --fail-with-body -sS "$ONE_API_BASE_URL/v1/videos/generations" \
  -H "Authorization: Bearer $ONE_API_KEY" \
  -H 'Content-Type: application/json' \
  --data-raw '{"model":"grok-imagine-video-1.5","prompt":"A paper boat floating on a quiet lake.","duration":5,"aspect_ratio":"16:9","resolution":"720p"}'
# {"request_id":"..."}

curl --fail-with-body -sS "$ONE_API_BASE_URL/v1/videos/$REQUEST_ID" \
  -H "Authorization: Bearer $ONE_API_KEY"
# {"status":"pending"}
# or {"status":"done","video":{"url":"...","duration":5,...},...}
```

Use the returned `request_id` as `REQUEST_ID`, retry polling at a bounded interval (for example five seconds), and stop on `done`, `failed`, `expired`, an HTTP error, or a client deadline. On `done`, download the temporary `video.url` directly; **do not send a one-api or xAI API key to that URL**. Do not append `/content` to the polling endpoint.

For image-to-video, add `"image":{"url":"https://your-public-host/input.png"}`. The Models modal provides text-to-video, image-to-video, and status curl examples at its bottom.

`POST /v1/videos` is an alternative gateway entrypoint for xAI channels; it uses the same **native xAI JSON response**, not an OpenAI/Sora response conversion. Native creation is preferred for SDK compatibility. Numeric `seconds` and `duration_seconds` are accepted as aliases for `duration`, but contradictory or invalid aliases are rejected before sending work upstream. Specify an integer duration of 1–15 seconds explicitly. The default resolution is made explicit as `480p`; supported resolution labels are `480p`, `720p`, and `1080p` (the provider may reject combinations unavailable for an older model or workflow). `size` accepts those resolution labels only, not pixel dimensions.

## Pricing and lifecycle

The existing video quota admission/settlement path is reused. It prices the same canonical duration/resolution that is physically sent upstream. Each supplied image/reference/frame adds the configured `input_image_usd` fee. Decimal arithmetic rounds up only the final quota value; binary floating-point noise does not add an extra quota unit. Model mapping, group rates, channel overrides, and trusted/unlimited-token accounting still apply.

The built-in Imagine Video 1.5 prices are $0.08/second at 480p, $0.14/second at 720p, $0.25/second at 1080p, plus $0.01 per input image. The classic model retains its own catalog rates and $0.002 per input image. Preset voice references do not add an image fee. Channel model configs can override the video block:

```json
{"grok-imagine-video-1.5":{"video":{"per_second_usd":0.08,"input_image_usd":0.01,"base_resolution":"480p","resolution_multipliers":{"480p":1,"720p":1.75,"1080p":3.125}}}}
```

A rejected HTTP creation or invalid creation envelope does not settle a successful generation charge; pre-consumed quota is refunded. A valid accepted receipt is billed even if the downstream connection breaks. Creation is **not automatically retried once it may have reached xAI**, because another POST could create a second paid job. A transport failure can be ambiguous; investigate instead of blindly recreating work.

Polling is quota-free and never settles the creation fee a second time. This change retains the gateway's existing charge-on-accepted-creation policy; it does not add a background billing/refund worker for a job that fails later. A later `failed` or `expired` status is surfaced as native status data, not falsely reported as a successful video.

Task reads require a persisted binding, the authenticated task owner, and an allowed model on the current token. They stay on the original channel. Unknown/foreign jobs return the same 404 rather than falling through to random channel selection; legacy jobs without a binding also fail closed. The shared task store logs persistence failures; operators must investigate such errors, and no unbound task is routable.

## Validation scope

Tests use local HTTP/TLS provider fixtures and real SQLite quota/task persistence, not paid xAI generation. They cover native wire requests/responses, aliases, mapped models, all resolution prices, image fees, channel overrides, admission/refunds, downstream disconnects, free repeated polling, owner/token isolation, retry safety, and the generated curl/UI contracts. These tests do not establish the availability of an operator's upstream account or validate actual generated video quality.
