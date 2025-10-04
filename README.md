# One API

## Synopsis

One‑API is a **single‑endpoint gateway** that lets you manage and call dozens of AI SaaS models without the headache of custom adapters. 🌐 Simply change the `model_name` and you can reach OpenAI, Anthropic, Gemini, Groq, DeepSeek, and many others—all through the same request format.

![](https://s3.laisky.com/uploads/2025/07/oneapi.drawio.png)

```plain
=== One-API Regression Report ===
│────────────────────│─────────────────────│────────────────────│─────────────────────────│────────────────────────│───────────────────────│──────────────────────│
│       MODEL        │ CHAT (STREAM=FALSE) │ CHAT (STREAM=TRUE) │ RESPONSE (STREAM=FALSE) │ RESPONSE (STREAM=TRUE) │ CLAUDE (STREAM=FALSE) │ CLAUDE (STREAM=TRUE) │
│────────────────────│─────────────────────│────────────────────│─────────────────────────│────────────────────────│───────────────────────│──────────────────────│
│ gpt-4o-mini        │ PASS                │ PASS               │ PASS                    │ PASS                   │ PASS                  │ PASS                 │
│                    │ 1.475s              │ 1.502s             │ 1.591s                  │ 1.115s                 │ 931ms                 │ 1.238s               │
│────────────────────│─────────────────────│────────────────────│─────────────────────────│────────────────────────│───────────────────────│──────────────────────│
│ gpt-5-mini         │ PASS                │ PASS               │ PASS                    │ PASS                   │ PASS                  │ PASS                 │
│                    │ 3.758s              │ 3.788s             │ 4.185s                  │ 7.194s                 │ 9.529s                │ 6.999s               │
│────────────────────│─────────────────────│────────────────────│─────────────────────────│────────────────────────│───────────────────────│──────────────────────│
│ claude-3.5-haiku   │ PASS                │ PASS               │ PASS                    │ PASS                   │ PASS                  │ PASS                 │
│                    │ 987ms               │ 693ms              │ 1.016s                  │ 773ms                  │ 1.266s                │ 704ms                │
│────────────────────│─────────────────────│────────────────────│─────────────────────────│────────────────────────│───────────────────────│──────────────────────│
│ gemini-2.5-flash   │ PASS                │ PASS               │ PASS                    │ PASS                   │ PASS                  │ PASS                 │
│                    │ 957ms               │ 913ms              │ 1.519s                  │ 1.369s                 │ 1.173s                │ 1.567s               │
│────────────────────│─────────────────────│────────────────────│─────────────────────────│────────────────────────│───────────────────────│──────────────────────│
│ openai/gpt-oss-20b │ PASS                │ PASS               │ PASS                    │ PASS                   │ PASS                  │ PASS                 │
│                    │ 625ms               │ 338ms              │ 476ms                   │ 506ms                  │ 567ms                 │ 537ms                │
│────────────────────│─────────────────────│────────────────────│─────────────────────────│────────────────────────│───────────────────────│──────────────────────│
│ deepseek-chat      │ PASS                │ PASS               │ PASS                    │ PASS                   │ PASS                  │ PASS                 │
│                    │ 2.679s              │ 1.326s             │ 2.444s                  │ 1.251s                 │ 1.969s                │ 3.627s               │
│────────────────────│─────────────────────│────────────────────│─────────────────────────│────────────────────────│───────────────────────│──────────────────────│

Totals  | Requests: 36 | Passed: 36 | Failed: 0 | Skipped: 0
```

### Why this fork exists

The original author stopped maintaining the project, leaving critical PRs and new features unaddressed. As a long‑time contributor, I’ve forked the repository and rebuilt the core to keep the ecosystem alive and evolving.

### What’s new

- **🔧 Complete billing overhaul** – per‑channel pricing for the same model, discounted rates for cached inputs, and transparent usage reports.
- **🖥️ Refreshed UI/UX** – a fully rewritten front‑end that makes tenant, quota, and cost management a breeze.
- **🔀 Transparent API conversion** – send a request in **ChatCompletion**, **Response**, or **Claude Messages** format and One‑API will automatically translate it to the target provider’s native schema.
- **🔄 Drop‑in database compatibility** – the original One‑API schema is fully supported, so you can migrate without data loss or schema changes.
- **🐳 Multi‑architecture support** – runs on Linux x86_64, ARM64, and Windows out of the box.

Docker images available on Docker Hub:

- `ppcelery/one-api:latest`
- `ppcelery/one-api:arm64-latest`

One‑API empowers developers to:

- Centralize **tenant management** and **quota control**.
- Route AI calls to the right model with a single endpoint.
- Track usage and costs in real time.

Also welcome to register and use my deployed one-api gateway, which supports various mainstream models. For usage instructions, please refer to <https://wiki.laisky.com/projects/gpt/pay/>.

- [One API](#one-api)
  - [Synopsis](#synopsis)
    - [Why this fork exists](#why-this-fork-exists)
    - [What’s new](#whats-new)
  - [Multi Agent Framework Compatible](#multi-agent-framework-compatible)
  - [›_ OpenCode Support](#_-opencode-support)
  - [Tutorial](#tutorial)
    - [Docker Compose Deployment](#docker-compose-deployment)
    - [Kubernetes Deployment](#kubernetes-deployment)
  - [Contributors](#contributors)
  - [New Features](#new-features)
    - [Universal Features](#universal-features)
      - [Support update user's remained quota](#support-update-users-remained-quota)
      - [Get request's cost](#get-requests-cost)
      - [Support Tracing info in logs](#support-tracing-info-in-logs)
      - [Support Cached Input](#support-cached-input)
        - [Support Anthropic Prompt caching](#support-anthropic-prompt-caching)
      - [Automatically Enable Thinking and Customize Reasoning Format via URL Parameters](#automatically-enable-thinking-and-customize-reasoning-format-via-url-parameters)
        - [Reasoning Format - reasoning-content](#reasoning-format---reasoning-content)
        - [Reasoning Format - reasoning](#reasoning-format---reasoning)
        - [Reasoning Format - thinking](#reasoning-format---thinking)
    - [OpenAI Features](#openai-features)
      - [(Merged) Support gpt-vision](#merged-support-gpt-vision)
      - [Support openai images edits](#support-openai-images-edits)
      - [Support OpenAI o1/o1-mini/o1-preview](#support-openai-o1o1-minio1-preview)
      - [Support gpt-4o-audio](#support-gpt-4o-audio)
      - [Support OpenAI web search models](#support-openai-web-search-models)
      - [Support gpt-image-1's image generation \& edits](#support-gpt-image-1s-image-generation--edits)
      - [Support o3-mini \& o3 \& o4-mini \& gpt-4.1 \& o3-pro \& reasoning content](#support-o3-mini--o3--o4-mini--gpt-41--o3-pro--reasoning-content)
      - [Support OpenAI Response API](#support-openai-response-api)
      - [Support gpt-5 family](#support-gpt-5-family)
      - [Support o3-deep-research \& o4-mini-deep-research](#support-o3-deep-research--o4-mini-deep-research)
      - [Support Codex Cli](#support-codex-cli)
    - [Anthropic (Claude) Features](#anthropic-claude-features)
      - [(Merged) Support aws claude](#merged-support-aws-claude)
      - [Support claude-3-7-sonnet \& thinking](#support-claude-3-7-sonnet--thinking)
        - [Stream](#stream)
        - [Non-Stream](#non-stream)
      - [Support /v1/messages Claude Messages API](#support-v1messages-claude-messages-api)
        - [Support Claude Code](#support-claude-code)
    - [Support claude-opus-4-0 / claude-opus-4-1 / claude-sonnet-4-0 / claude-sonnet-4-5](#support-claude-opus-4-0--claude-opus-4-1--claude-sonnet-4-0--claude-sonnet-4-5)
    - [Google (Gemini \& Vertex) Features](#google-gemini--vertex-features)
      - [Support gemini-2.0-flash-exp](#support-gemini-20-flash-exp)
      - [Support gemini-2.0-flash](#support-gemini-20-flash)
      - [Support gemini-2.0-flash-thinking-exp-01-21](#support-gemini-20-flash-thinking-exp-01-21)
      - [Support Vertex Imagen3](#support-vertex-imagen3)
      - [Support gemini multimodal output #2197](#support-gemini-multimodal-output-2197)
      - [Support gemini-2.5-pro](#support-gemini-25-pro)
      - [Support GCP Vertex gloabl region and gemini-2.5-pro-preview-06-05](#support-gcp-vertex-gloabl-region-and-gemini-25-pro-preview-06-05)
      - [Support gemini-2.5-flash-image-preview \& imagen-4 series](#support-gemini-25-flash-image-preview--imagen-4-series)
    - [AWS Features](#aws-features)
      - [Support AWS cross-region inferences](#support-aws-cross-region-inferences)
      - [Support AWS BedRock Inference Profile](#support-aws-bedrock-inference-profile)
    - [Replicate Features](#replicate-features)
      - [Support replicate flux \& remix](#support-replicate-flux--remix)
      - [Support replicate chat models](#support-replicate-chat-models)
    - [DeepSeek Features](#deepseek-features)
      - [Support deepseek-reasoner](#support-deepseek-reasoner)
    - [OpenRouter Features](#openrouter-features)
      - [Support OpenRouter's reasoning content](#support-openrouters-reasoning-content)
    - [Coze Features](#coze-features)
      - [Support coze oauth authentication](#support-coze-oauth-authentication)
    - [XAI / Grok Features](#xai--grok-features)
      - [Support XAI/Grok Text \& Image Models](#support-xaigrok-text--image-models)
    - [Black Forest Labs Features](#black-forest-labs-features)
      - [Support black-forest-labs/flux-kontext-pro](#support-black-forest-labsflux-kontext-pro)
  - [Bug Fixes \& Enterprise-Grade Improvements (Including Security Enhancements)](#bug-fixes--enterprise-grade-improvements-including-security-enhancements)

## Multi Agent Framework Compatible

This repository is fully compatible with multi-agent frameworks and is recommended for use with chat completion OpenAI compatible APIs. The unified interface provided by One API makes it an ideal choice for integrating multiple AI services into multi-agent systems, allowing agents to seamlessly interact with various AI models through a standardized OpenAI-compatible endpoint.

> [!TIP]
> For optimal performance in multi-agent environments, it's recommended to use models that already have automated cached prompt capabilities, such as `grok-code-fast-1`. These models can significantly reduce latency and improve response times by leveraging cached prompts, which is especially beneficial when multiple agents are making frequent requests with similar contexts.

## ›_ OpenCode Support

<p align="center">
  <a href="https://opencode.ai">
    <picture>
      <source srcset="https://github.com/sst/opencode/raw/dev/packages/console/app/src/asset/logo-ornate-dark.svg" media="(prefers-color-scheme: dark)">
      <source srcset="https://github.com/sst/opencode/raw/dev/packages/console/app/src/asset/logo-ornate-light.svg" media="(prefers-color-scheme: light)">
      <img src="https://github.com/sst/opencode/raw/dev/packages/console/app/src/asset/logo-ornate-light.svg" alt="OpenCode logo">
    </picture>
  </a>
</p>

[opencode.ai](https://opencode.ai) is an AI coding agent built for the terminal. OpenCode is fully open source, giving you control and `freedom` to use any provider, any model, and any editor. It's available as both a CLI and TUI.

One‑API integrates seamlessly with OpenCode: you can connect any One‑API endpoint and use all your unified models through OpenCode's interface (both CLI and TUI).

To get started, create or edit `~/.config/opencode/opencode.json` like this:

**Using OpenAI SDK:**

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "one-api": {
      "npm": "@ai-sdk/openai",
      "name": "One API",
      "options": {
        "baseURL": "http://localhost:3000/v1",
        "apiKey":  "HANDLE_APIKEY_HERE"
      },
      "models": {
        "gpt-4.1-2025-04-14": {
          "name": "GPT 4.1"
        }
      }
    }
  }
}
```

**Using Anthropic SDK:**

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "one-api-anthropic": {
      "npm": "@ai-sdk/anthropic",
      "name": "One API (Anthropic)",
      "options": {
        "baseURL": "http://localhost:3000/v1",
        "apiKey":  "HANDLE_APIKEY_HERE"
      },
      "models": {
        "claude-sonnet-4-5": {
          "name": "Claude Sonnet 4.5"
        }
      }
    }
  }
}
```

- Replace `HANDLE_APIKEY_HERE` with your One‑API key.
- Make sure `baseURL` matches your running One‑API endpoint.
- Now, any opencode command (like `opencode chat`, `opencode run`, and more) works with all your One‑API models directly from the terminal! 😎

**Best Practice - Using Environment Variables:**

For better security, use environment variables instead of hardcoding API keys:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "one-api": {
      "npm": "@ai-sdk/openai",
      "name": "One API",
      "options": {
        "baseURL": "http://localhost:3000/v1",
        "apiKey": "{env:ONEAPI_API_KEY}"
      },
      "models": {
        "gpt-4.1-2025-04-14": {
          "name": "GPT 4.1"
        }
      }
    }
  }
}
```

Then set the environment variable in your shell:

```bash
export ONEAPI_API_KEY="sk-your-actual-api-key-here"
```

## Tutorial

### Docker Compose Deployment

Run one-api using docker-compose:

```yaml
oneapi:
  image: ppcelery/one-api:latest
  restart: unless-stopped
  logging:
    driver: "json-file"
    options:
      max-size: "10m"
  environment:
    # (optional) SESSION_SECRET set a fixed session secret so that user sessions won't be invalidated after server restart
    SESSION_SECRET: xxxxxxx
    # (optional) If you access one-api using a non-HTTPS address, you need to set DISABLE_COOKIE_SECURE to true
    DISABLE_COOKIE_SECURE: "true"

    # (optional) DEBUG enable debug mode
    DEBUG: "true"

    # (optional) DEBUG_SQL display SQL logs
    DEBUG_SQL: "true"
    # (optional) REDIS_CONN_STRING set REDIS cache connection
    REDIS_CONN_STRING: redis://100.122.41.16:6379/1
    # (optional) SQL_DSN set SQL database connection,
    # default is sqlite3, support mysql, postgresql, sqlite3
    SQL_DSN: "postgres://laisky:xxxxxxx@1.2.3.4/oneapi"

    # (optional) ENFORCE_INCLUDE_USAGE require upstream API responses to include usage field
    ENFORCE_INCLUDE_USAGE: "true"

    # (optional) MAX_ITEMS_PER_PAGE maximum items per page, default is 10
    MAX_ITEMS_PER_PAGE: 10

    # (optional) GLOBAL_API_RATE_LIMIT maximum API requests per IP within three minutes, default is 1000
    GLOBAL_API_RATE_LIMIT: 1000
    # (optional) GLOBAL_WEB_RATE_LIMIT maximum web page requests per IP within three minutes, default is 1000
    GLOBAL_WEB_RATE_LIMIT: 1000
    # (optional) /v1 API ratelimit for each token
    GLOBAL_RELAY_RATE_LIMIT: 1000
    # (optional) Whether to ratelimit for channel, 0 is unlimited, 1 is to enable the ratelimit
    GLOBAL_CHANNEL_RATE_LIMIT: 1
    # (optional) ShutdownTimeoutSec controls how long to wait for graceful shutdown and drains (seconds)
    SHUTDOWN_TIMEOUT_SEC: 360

    # (optional) FRONTEND_BASE_URL redirect page requests to specified address, server-side setting only
    FRONTEND_BASE_URL: https://oneapi.laisky.com

    # (optional) OPENROUTER_PROVIDER_SORT set sorting method for OpenRouter Providers, default is throughput
    OPENROUTER_PROVIDER_SORT: throughput

    # (optional) CHANNEL_SUSPEND_SECONDS_FOR_429 set the duration for channel suspension when receiving 429 error, default is 60 seconds
    CHANNEL_SUSPEND_SECONDS_FOR_429: 60

    # (optional) DEFAULT_MAX_TOKEN set the default maximum number of tokens for requests, default is 2048
      DEFAULT_MAX_TOKEN: 2048
    # (optional) MAX_INLINE_IMAGE_SIZE_MB set the maximum allowed image size (in MB) for inlining images as base64, default is 30
      MAX_INLINE_IMAGE_SIZE_MB: 30

    # (optional) LOG_PUSH_API set the API address for pushing error logs to telegram.
    # More information about log push can be found at: https://github.com/Laisky/laisky-blog-graphql/tree/master/internal/web/telegram
    LOG_PUSH_API: "https://gq.laisky.com/query/"
    LOG_PUSH_TYPE: "oneapi"
    LOG_PUSH_TOKEN: "xxxxxxx"

  volumes:
    - /var/lib/oneapi:/data
  ports:
    - 3000:3000
```

The initial default account and password are `root` / `123456`.

> [!TIP] > **Secret Management**: For production environments, consider using proper secret management solutions instead of hardcoding sensitive values in environment variables:

### Kubernetes Deployment

The Kubernetes deployment guide has been moved into a dedicated document:

- [docs/manuals/k8s.md](docs/manuals/k8s.md)

## Contributors

<a href="https://github.com/Laisky/one-api/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=Laisky/one-api" />
</a>

## New Features

### Universal Features

#### Support update user's remained quota

You can update the used quota using the API key of any token, allowing other consumption to be aggregated into the one-api for centralized management.

![](https://s3.laisky.com/uploads/2024/12/oneapi-update-quota.png)

#### Get request's cost

Each chat completion request will include a `X-Oneapi-Request-Id` in the returned headers. You can use this request id to request `GET /api/cost/request/:request_id` to get the cost of this request.

The returned structure is:

```go
type UserRequestCost struct {
  Id          int     `json:"id"`
  CreatedTime int64   `json:"created_time" gorm:"bigint"`
  UserID      int     `json:"user_id"`
  RequestID   string  `json:"request_id"`
  Quota       int64   `json:"quota"`
  CostUSD     float64 `json:"cost_usd" gorm:"-"`
}
```

#### Support Tracing info in logs

![](https://s3.laisky.com/uploads/2025/08/tracing.png)

#### Support Cached Input

Now supports cached input, which can significantly reduce the cost.

![](https://s3.laisky.com/uploads/2025/08/cached_input.png)

##### Support Anthropic Prompt caching

- <https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching>

#### Automatically Enable Thinking and Customize Reasoning Format via URL Parameters

Supports two URL parameters: `thinking` and `reasoning_format`.

- `thinking`: Whether to enable thinking mode, disabled by default.
- `reasoning_format`: Specifies the format of the returned reasoning.
  - `reasoning_content`: DeepSeek official API format, returned in the `reasoning_content` field.
  - `reasoning`: OpenRouter format, returned in the `reasoning` field.
  - `thinking`: Claude format, returned in the `thinking` field.

##### Reasoning Format - reasoning-content

![](https://s3.laisky.com/uploads/2025/02/reasoning_format-reasoning_content.png)

##### Reasoning Format - reasoning

![](https://s3.laisky.com/uploads/2025/02/reasoning_format-reasoning.png)

##### Reasoning Format - thinking

![](https://s3.laisky.com/uploads/2025/02/reasoning_format-thinking.png)

### OpenAI Features

#### (Merged) Support gpt-vision

#### Support openai images edits

- [feat: support openai images edits api #1369](https://github.com/songquanpeng/one-api/pull/1369)

![](https://s3.laisky.com/uploads/2024/12/oneapi-image-edit.png)

#### Support OpenAI o1/o1-mini/o1-preview

- [feat: add openai o1 #1990](https://github.com/songquanpeng/one-api/pull/1990)

#### Support gpt-4o-audio

- [feat: support gpt-4o-audio #2032](https://github.com/songquanpeng/one-api/pull/2032)

![](https://s3.laisky.com/uploads/2025/01/oneapi-audio-1.png)

![](https://s3.laisky.com/uploads/2025/01/oneapi-audio-2.png)

#### Support OpenAI web search models

- [feature: support openai web search models #2189](https://github.com/songquanpeng/one-api/pull/2189)

support `gpt-4o-search-preview` & `gpt-4o-mini-search-preview`

![](https://s3.laisky.com/uploads/2025/03/openai-websearch-models-1.png)

![](https://s3.laisky.com/uploads/2025/03/openai-websearch-models-2.png)

#### Support gpt-image-1's image generation & edits

![](https://s3.laisky.com/uploads/2025/04/gpt-image-1-2.png)

![](https://s3.laisky.com/uploads/2025/04/gpt-image-1-3.png)

![](https://s3.laisky.com/uploads/2025/04/gpt-image-1-1.png)

#### Support o3-mini & o3 & o4-mini & gpt-4.1 & o3-pro & reasoning content

- [feat: extend support for o3 models and update model ratios #2048](https://github.com/songquanpeng/one-api/pull/2048)

![](https://s3.laisky.com/uploads/2025/06/o3-pro.png)

#### Support OpenAI Response API

**Partially supported, still in development.**

![](https://s3.laisky.com/uploads/2025/07/response-api.png)

#### Support gpt-5 family

gpt-5-chat-latest / gpt-5 / gpt-5-mini / gpt-5-nano / gpt-5-codex

#### Support o3-deep-research & o4-mini-deep-research

![](https://s3.laisky.com/uploads/2025/09/o4-mini-deep-research.png)

#### Support Codex Cli

```sh
# vi $HOME/.codex/config.toml

model = "gemini-2.5-flash"
model_provider = "laisky"

[model_providers.laisky]
# Name of the provider that will be displayed in the Codex UI.
name = "Laisky"
# The path `/chat/completions` will be amended to this URL to make the POST
# request for the chat completions.
base_url = "https://oneapi.laisky.com/v1"
# If `env_key` is set, identifies an environment variable that must be set when
# using Codex with this provider. The value of the environment variable must be
# non-empty and will be used in the `Bearer TOKEN` HTTP header for the POST request.
env_key = "sk-xxxxxxx"
# Valid values for wire_api are "chat" and "responses". Defaults to "chat" if omitted.
wire_api = "responses"
# If necessary, extra query params that need to be added to the URL.
# See the Azure example below.
query_params = {}

```

### Anthropic (Claude) Features

#### (Merged) Support aws claude

- [feat: support aws bedrockruntime claude3 #1328](https://github.com/songquanpeng/one-api/pull/1328)
- [feat: add new claude models #1910](https://github.com/songquanpeng/one-api/pull/1910)

![](https://s3.laisky.com/uploads/2024/12/oneapi-claude.png)

#### Support claude-3-7-sonnet & thinking

- [feat: support claude-3-7-sonnet #2143](https://github.com/songquanpeng/one-api/pull/2143/files)
- [feat: support claude thinking #2144](https://github.com/songquanpeng/one-api/pull/2144)

By default, the thinking mode is not enabled. You need to manually pass the `thinking` field in the request body to enable it.

##### Stream

![](https://s3.laisky.com/uploads/2025/02/claude-thinking.png)

##### Non-Stream

![](https://s3.laisky.com/uploads/2025/02/claude-thinking-non-stream.png)

#### Support /v1/messages Claude Messages API

![](https://s3.laisky.com/uploads/2025/07/claude_messages.png)

##### Support Claude Code

```sh
export ANTHROPIC_MODEL="openai/gpt-oss-120b"
export ANTHROPIC_BASE_URL="https://oneapi.laisky.com/"
export ANTHROPIC_AUTH_TOKEN="sk-xxxxxxx"
```

You can use any model you like for Claude Code, even if the model doesn’t natively support the Claude Messages API.

### Support claude-opus-4-0 / claude-opus-4-1 / claude-sonnet-4-0 / claude-sonnet-4-5

![](https://s3.laisky.com/uploads/2025/09/claude-sonnet-4-5.png)

### Google (Gemini & Vertex) Features

#### Support gemini-2.0-flash-exp

- [feat: add gemini-2.0-flash-exp #1983](https://github.com/songquanpeng/one-api/pull/1983)

![](https://s3.laisky.com/uploads/2024/12/oneapi-gemini-flash.png)

#### Support gemini-2.0-flash

- [feat: support gemini-2.0-flash #2055](https://github.com/songquanpeng/one-api/pull/2055)

#### Support gemini-2.0-flash-thinking-exp-01-21

- [feature: add deepseek-reasoner & gemini-2.0-flash-thinking-exp-01-21 #2045](https://github.com/songquanpeng/one-api/pull/2045)

#### Support Vertex Imagen3

- [feat: support vertex imagen3 #2030](https://github.com/songquanpeng/one-api/pull/2030)

![](https://s3.laisky.com/uploads/2025/01/oneapi-imagen3.png)

#### Support gemini multimodal output #2197

- [feature: support gemini multimodal output #2197](https://github.com/songquanpeng/one-api/pull/2197)

![](https://s3.laisky.com/uploads/2025/03/gemini-multimodal.png)

#### Support gemini-2.5-pro

#### Support GCP Vertex gloabl region and gemini-2.5-pro-preview-06-05

![](https://s3.laisky.com/uploads/2025/06/gemini-2.5-pro-preview-06-05.png)

#### Support gemini-2.5-flash-image-preview & imagen-4 series

![](https://s3.laisky.com/uploads/2025/09/gemini-banana.png)

### AWS Features

#### Support AWS cross-region inferences

- [fix: support aws cross region inferences #2182](https://github.com/songquanpeng/one-api/pull/2182)

#### Support AWS BedRock Inference Profile

![](https://s3.laisky.com/uploads/2025/07/aws-inference-profile.png)

### Replicate Features

#### Support replicate flux & remix

- [feature: 支持 replicate 的绘图 #1954](https://github.com/songquanpeng/one-api/pull/1954)
- [feat: image edits/inpaiting 支持 replicate 的 flux remix #1986](https://github.com/songquanpeng/one-api/pull/1986)

![](https://s3.laisky.com/uploads/2024/12/oneapi-replicate-1.png)

![](https://s3.laisky.com/uploads/2024/12/oneapi-replicate-2.png)

![](https://s3.laisky.com/uploads/2024/12/oneapi-replicate-3.png)

#### Support replicate chat models

- [feat: 支持 replicate chat models #1989](https://github.com/songquanpeng/one-api/pull/1989)

### DeepSeek Features

#### Support deepseek-reasoner

- [feature: add deepseek-reasoner & gemini-2.0-flash-thinking-exp-01-21 #2045](https://github.com/songquanpeng/one-api/pull/2045)

### OpenRouter Features

#### Support OpenRouter's reasoning content

- [feat: support OpenRouter reasoning #2108](https://github.com/songquanpeng/one-api/pull/2108)

By default, the thinking mode is automatically enabled for the deepseek-r1 model, and the response is returned in the open-router format.

![](https://s3.laisky.com/uploads/2025/02/openrouter-reasoning.png)

### Coze Features

#### Support coze oauth authentication

- [feat: support coze oauth authentication](https://github.com/Laisky/one-api/pull/52)

### XAI / Grok Features

#### Support XAI/Grok Text & Image Models

![](https://s3.laisky.com/uploads/2025/08/groq.png)

### Black Forest Labs Features

#### Support black-forest-labs/flux-kontext-pro

![](https://s3.laisky.com/uploads/2025/05/flux-kontext-pro.png)

## Bug Fixes & Enterprise-Grade Improvements (Including Security Enhancements)

- [BUGFIX: Several issues when updating tokens #1933](https://github.com/songquanpeng/one-api/pull/1933)
- [feat(audio): count whisper-1 quota by audio duration #2022](https://github.com/songquanpeng/one-api/pull/2022)
- [fix: Fix issue where high-quota users using low-quota tokens aren't pre-charged, causing large token deficits under high concurrency #25](https://github.com/Laisky/one-api/pull/25)
- [fix: channel test false negative #2065](https://github.com/songquanpeng/one-api/pull/2065)
- [fix: resolve "bufio.Scanner: token too long" error by increasing buffer size #2128](https://github.com/songquanpeng/one-api/pull/2128)
- [feat: Enhance VolcEngine channel support with bot model #2131](https://github.com/songquanpeng/one-api/pull/2131)
- [fix: models API returns models in deactivated channels #2150](https://github.com/songquanpeng/one-api/pull/2150)
- [fix: Automatically close channel when connection fails](https://github.com/Laisky/one-api/pull/34)
- [fix: update EmailDomainWhitelist submission logic #33](https://github.com/Laisky/one-api/pull/33)
- [fix: send ByAll](https://github.com/Laisky/one-api/pull/35)
- [fix: oidc token endpoint request body #2106 #36](https://github.com/Laisky/one-api/pull/36)

> [!NOTE]
>
> For additional enterprise-grade improvements, including security enhancements (e.g., [vulnerability fixes](https://github.com/Laisky/one-api/pull/126)), you can also view these pull requests [here](https://github.com/Laisky/one-api/pulls?q=is%3Apr+is%3Aclosed).
