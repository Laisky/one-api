# One API

The original author of one-api has not been active for a long time, resulting in a backlog of PRs that cannot be updated. Therefore, I forked the code and merged some PRs that I consider important. I also welcome everyone to submit PRs, and I will respond and handle them actively and quickly.

Fully compatible with the upstream version, can be used directly by replacing the container image, docker images:

- `ppcelery/one-api:latest`
- `ppcelery/one-api:arm64-latest`

Also welcome to register and use my deployed one-api gateway, which supports various mainstream models. For usage instructions, please refer to <https://wiki.laisky.com/projects/gpt/pay/cn/#page_gpt_pay_cn>.

- [One API](#one-api)
  - [New Features](#new-features)
    - [(Merged) Support gpt-vision](#merged-support-gpt-vision)
    - [Support update user's remained quota](#support-update-users-remained-quota)
    - [(Merged) Support aws claude](#merged-support-aws-claude)
    - [Support openai images edits](#support-openai-images-edits)
    - [Support gemini-2.0-flash-exp](#support-gemini-20-flash-exp)
    - [Support replicate flux \& remix](#support-replicate-flux--remix)
    - [Support replicate chat models](#support-replicate-chat-models)
    - [Support OpenAI o1/o1-mini/o1-preview](#support-openai-o1o1-minio1-preview)
    - [Get request's cost](#get-requests-cost)
    - [Support Vertex Imagen3](#support-vertex-imagen3)
    - [Support gpt-4o-audio](#support-gpt-4o-audio)
    - [Support deepseek-reasoner \& gemini-2.0-flash-thinking-exp-01-21](#support-deepseek-reasoner--gemini-20-flash-thinking-exp-01-21)
    - [Support o3-mini](#support-o3-mini)
    - [Support gemini-2.0-flash](#support-gemini-20-flash)
    - [Support OpenRouter's reasoning content](#support-openrouters-reasoning-content)
  - [Bug fix](#bug-fix)

## New Features

### (Merged) Support gpt-vision

### Support update user's remained quota

You can update the used quota using the API key of any token, allowing other consumption to be aggregated into the one-api for centralized management.

![](https://s3.laisky.com/uploads/2024/12/oneapi-update-quota.png)

### (Merged) Support aws claude

- [feat: support aws bedrockruntime claude3 #1328](https://github.com/songquanpeng/one-api/pull/1328)
- [feat: add new claude models #1910](https://github.com/songquanpeng/one-api/pull/1910)

![](https://s3.laisky.com/uploads/2024/12/oneapi-claude.png)

### Support openai images edits

- [feat: support openai images edits api #1369](https://github.com/songquanpeng/one-api/pull/1369)

![](https://s3.laisky.com/uploads/2024/12/oneapi-image-edit.png)

### Support gemini-2.0-flash-exp

- [feat: add gemini-2.0-flash-exp #1983](https://github.com/songquanpeng/one-api/pull/1983)

![](https://s3.laisky.com/uploads/2024/12/oneapi-gemini-flash.png)

### Support replicate flux & remix

- [feature: 支持 replicate 的绘图 #1954](https://github.com/songquanpeng/one-api/pull/1954)
- [feat: image edits/inpaiting 支持 replicate 的 flux remix #1986](https://github.com/songquanpeng/one-api/pull/1986)

![](https://s3.laisky.com/uploads/2024/12/oneapi-replicate-1.png)

![](https://s3.laisky.com/uploads/2024/12/oneapi-replicate-2.png)

![](https://s3.laisky.com/uploads/2024/12/oneapi-replicate-3.png)

### Support replicate chat models

- [feat: 支持 replicate chat models #1989](https://github.com/songquanpeng/one-api/pull/1989)

### Support OpenAI o1/o1-mini/o1-preview

- [feat: add openai o1 #1990](https://github.com/songquanpeng/one-api/pull/1990)

### Get request's cost

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

### Support Vertex Imagen3

- [feat: support vertex imagen3 #2030](https://github.com/songquanpeng/one-api/pull/2030)

![](https://s3.laisky.com/uploads/2025/01/oneapi-imagen3.png)

### Support gpt-4o-audio

- [feat: support gpt-4o-audio #2032](https://github.com/songquanpeng/one-api/pull/2032)

![](https://s3.laisky.com/uploads/2025/01/oneapi-audio-1.png)

![](https://s3.laisky.com/uploads/2025/01/oneapi-audio-2.png)

### Support deepseek-reasoner & gemini-2.0-flash-thinking-exp-01-21

- [feature: add deepseek-reasoner & gemini-2.0-flash-thinking-exp-01-21 #2045](https://github.com/songquanpeng/one-api/pull/2045)

### Support o3-mini

- [feat: extend support for o3 models and update model ratios #2048](https://github.com/songquanpeng/one-api/pull/2048)

### Support gemini-2.0-flash

- [feat: support gemini-2.0-flash #2055](https://github.com/songquanpeng/one-api/pull/2055)

### Support OpenRouter's reasoning content

- [feat: support OpenRouter reasoning #2108](https://github.com/songquanpeng/one-api/pull/2108)

## Bug fix

- [BUGFIX: 更新令牌时的一些问题 #1933](https://github.com/songquanpeng/one-api/pull/1933)
- [feat(audio): count whisper-1 quota by audio duration #2022](https://github.com/songquanpeng/one-api/pull/2022)
- [fix: 修复高并发下，高额度用户使用低额度令牌没有预扣费而导致令牌大额欠费 #25](https://github.com/Laisky/one-api/pull/25)
- [fix: channel test false negative #2065](https://github.com/songquanpeng/one-api/pull/2065)
