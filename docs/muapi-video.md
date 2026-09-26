# MuAPI video generation

The MuAPI channel exposes MuAPI's model-scoped asynchronous video API through
one-api's OpenAI-compatible video lifecycle:

- `POST /v1/videos` or `POST /v1/videos/generations` submits a job.
- `GET /v1/videos/{request_id}` polls the job.
- MuAPI model slugs are placed in the upstream path, so the adaptor does not
  maintain a hard-coded allowlist. Configure any video model currently listed
  by MuAPI in the channel's Models field.

## Channel setup

Create a channel with:

- Type: `MuAPI`
- Base URL: `https://api.muapi.ai`
- Key: a MuAPI API key
- Models: one or more current MuAPI video model slugs, comma-separated
- Endpoint: `videos`

MuAPI's catalog and schemas are published at
<https://api.muapi.ai/api/v1/models>. The public documentation describes the
submit endpoint as `POST /api/v1/{model}` and the polling endpoint as
`GET /api/v1/predictions/{request_id}/result`.

## Request example

```sh
curl "$ONE_API_URL/v1/videos" \
  -H "Authorization: Bearer $ONE_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "veo3-fast",
    "prompt": "A slow cinematic drone shot over a mountain lake at sunrise",
    "duration": 8,
    "aspect_ratio": "16:9"
  }'
```

The response keeps MuAPI's native `request_id` and `status`. Poll with the
returned id until the native status is `completed` or `failed`:

```sh
curl "$ONE_API_URL/v1/videos/$REQUEST_ID" \
  -H "Authorization: Bearer $ONE_API_TOKEN"
```

Provider-specific fields are passed through after the gateway removes the
model field (the model is encoded in the upstream path) and normalizes
`seconds`/`duration_seconds` to `duration`. Use the model schema from MuAPI for
additional image, reference, audio, resolution, or motion-control fields.

## Pricing and safety

MuAPI prices many media models dynamically by model and request parameters. The
adaptor calls the documented `estimate-cost` endpoint before one-api reserves
quota, converts the returned USD cost into the gateway's per-second billing
contract for that request, and refuses to submit when a positive USD estimate
is unavailable. No generation request is replayed automatically.

Accepted request ids are bound to the authenticated user, token, model, and
channel through one-api's shared async-video task store. Listing, deletion, and
`/content` routes are intentionally not emulated; download the output URL from
MuAPI's completed result.

References:

- <https://muapi.ai/about>
- <https://muapi.ai/ai-video-api>
- <https://muapi.ai/de/docs/pricing>
- <https://muapi.ai/docs/authentication>
