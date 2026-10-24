# Moonshot/Kimi model audit and update HOWTO — 2026-10-24

## Scope and sources

Refresh the native Moonshot catalogue from official documentation, including the
new K3.1 Pro models, documented token budgets, parameter restrictions, and CNY
price defaults. Price metadata does not by itself implement cache-write usage
normalization or settlement. This audit makes no claim of live API qualification;
the implementation task record contains validation evidence.

Primary sources, checked on 2026-10-24:

- [Current model catalogue](https://platform.kimi.com/docs/models)
- [Domestic CNY pricing](https://platform.kimi.com/docs/pricing/chat)
- [International USD pricing](https://platform.kimi.ai/docs/pricing/chat)
- [K3.1 Pro guide](https://platform.kimi.com/docs/guide/kimi-k3-1-pro-quickstart)
- [K3 guide](https://platform.kimi.com/docs/guide/kimi-k3-quickstart)
- [K2.7 Code guide](https://platform.kimi.com/docs/guide/kimi-k2-7-code-quickstart)
- [K2.6 guide](https://platform.kimi.com/docs/guide/kimi-k2-6-quickstart)
- [Parameter reference](https://platform.kimi.ai/docs/api/models-overview)
- [Chat Completions API schema](https://platform.kimi.ai/docs/api/chat)
- [Thinking guide](https://platform.kimi.ai/docs/guide/use-thinking-models)
- [Output-budget troubleshooting](https://platform.kimi.com/docs/guide/troubleshooting)
- [Cache accounting](https://platform.kimi.ai/docs/guide/context-caching)

The former `platform.moonshot.cn` and `platform.moonshot.ai` documentation URLs
redirect to `platform.kimi.com` and `platform.kimi.ai`. API examples still use
`api.moonshot.cn` and `api.moonshot.ai`; a documentation redirect is not evidence
that configured API endpoints should be rewritten.

## Current models and native price defaults

These are the six current model IDs. All support text, image and video input,
reasoning, and tool calls. Amounts are **CNY per 1,000,000 tokens**, taken directly
from the domestic tariff. A dash means no separate cache-write tariff is listed.

| Model | Context tokens | Input | Output | Cache read | Write 5m | Write 1h |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `kimi-k3.1-pro` | 1,048,576 | 13.00 | 68.00 | 1.30 | 13.00 | 26.00 |
| `kimi-k3.1-pro-highspeed` | 1,048,576 | 26.00 | 136.00 | 1.30 | 26.00 | 52.00 |
| `kimi-k3` | 1,048,576 | 20.00 | 100.00 | 2.00 | 20.00 | 40.00 |
| `kimi-k2.7-code` | 262,144 | 6.50 | 27.00 | 1.30 | — | — |
| `kimi-k2.7-code-highspeed` | 262,144 | 13.00 | 54.00 | 2.60 | — | — |
| `kimi-k2.6` | 262,144 | 6.50 | 27.00 | 1.10 | — | — |

### Independent international reference

The international tariff is **USD per 1,000,000 tokens**, not an exchange-rate
conversion of the domestic tariff. Do not substitute these prices for native CNY
defaults or infer prices for another provider serving Kimi weights.

| Model | Input | Output | Cache read | Write 5m | Write 1h |
| --- | ---: | ---: | ---: | ---: | ---: |
| `kimi-k3.1-pro` | 2.00 | 10.00 | 0.20 | 2.00 | 4.00 |
| `kimi-k3.1-pro-highspeed` | 4.00 | 20.00 | 0.20 | 4.00 | 8.00 |
| `kimi-k3` | 3.00 | 15.00 | 0.30 | 3.00 | 6.00 |
| `kimi-k2.7-code` | 0.95 | 4.00 | 0.19 | — | — |
| `kimi-k2.7-code-highspeed` | 1.90 | 8.00 | 0.38 | — | — |
| `kimi-k2.6` | 0.95 | 4.00 | 0.16 | — | — |

## Budgets and parameter restrictions

- K3 models use a 1,048,576-token context and maximum completion budget; the
  upstream default completion budget is 131,072. Input consumes context space,
  and the API reduces generation to fit the remaining context.
- K2.6 and K2.7 Code models use a 262,144-token context. Their maximum output is
  the remaining context (`262144 - prompt_tokens`), while 32,768 is the upstream
  default, not a hard ceiling. Reasoning and final content share that budget;
  no independent 49,152-token reasoning ceiling is documented.
- The current Chat Completions schema includes `max_completion_tokens` for K2.6
  and both K2.7 Code variants as well as K3; it marks `max_tokens` deprecated.
  K2 guides continue to show `max_tokens` examples, so retain legacy compatibility.
- K3 always reasons, preserves historical thinking, and accepts top-level
  `reasoning_effort` values `low`, `high`, and `max` (default `max`). It does not
  use the K2 `thinking` parameter and supports `tool_choice=required`.
- K2.7 Code always thinks and preserves thinking. K2.6 defaults to thinking but
  supports `thinking.type=disabled`; `thinking.keep=all` preserves past reasoning.
  Neither K2 family accepts `reasoning_effort` or `tool_choice=required`.
- All six fix `top_p=0.95`, `n=1`, and presence/frequency penalties at zero.
  Temperature is fixed at 1.0, except K2.6 nonthinking mode uses 0.6. Other values
  cause upstream errors; official guidance is to omit fixed sampling parameters.
  Each highspeed variant shares its standard model's parameter constraints.

## Retirement and compatibility

From **2026-10-31**, remaining K2 models stop accepting newly registered users;
existing users are unaffected at that date. Full K2 retirement is planned for
the end of December 2026. This is a provider notice, not a local routing cutoff.

The provider retired K2.5 and all `moonshot-v1` variants on 2026-08-31, the older
K2 preview/thinking models on 2026-05-25, `kimi-latest` on 2026-01-28, and
`kimi-thinking-preview` on 2025-11-11. Preserve existing legacy catalogue entries,
historical amounts and operator configurations; do not remap model names or add
new historical aliases as current offerings. A cache FAQ abbreviates K2.7 names
without `-code`; the catalogue's explicit IDs above are authoritative.

## Cache accounting boundary

For Chat Completions, input tokens consist of mutually exclusive uncached,
cache-read, and cache-write portions. A 5-minute write costs the same as normal
input; charging those write tokens once as uncached input already yields the
documented total. Adding another write fee on top would double-charge them.

One-hour writes cost twice normal input. The existing shared parser does not
normalize Moonshot's nested `prompt_tokens_details.cache_write_tokens` and TTL
headers into native settlement; recording tariff metadata does not close that
gap. The headers are `Msh-Usage-Cache-Write-Tokens-5m` and
`Msh-Usage-Cache-Write-Tokens-1h`. Existing handling can undercharge 1h writes, so
do not claim complete native 1h billing support from this catalogue refresh.
Streaming usage requires `stream_options.include_usage=true` upstream. A later
accounting change needs behavior tests for each receipt and TTL combination.

## HOWTO: verify the numeric tariff before another refresh

1. Recheck the official catalogue, model guides, parameter reference and both
   regional tariffs. Distinguish a default output budget from a maximum.
2. If Markdown or rendered-text extraction has empty tables, inspect the public
   pricing HTML's embedded compiled table. The following one-off research command
   prints exact model rows and saves raw evidence under `/tmp`, outside the repo:

```sh
python - <<'PY'
from pathlib import Path
import hashlib
import re
import urllib.request

out = Path('/tmp/kimi-model-pricing-recheck')
out.mkdir(exist_ok=True)
for host in ('platform.kimi.com', 'platform.kimi.ai'):
    url = f'https://{host}/docs/pricing/chat'
    request = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
    with urllib.request.urlopen(request, timeout=30) as response:
        raw = response.read()
    (out / f'{host}.html').write_bytes(raw)
    print(url, hashlib.sha256(raw).hexdigest())
    pattern = r'children:`(kimi-[^`]+)`\}\),([^\]]+)\]'
    rows = list(re.finditer(pattern, raw.decode('utf-8')))
    if not rows:
        raise RuntimeError('Pricing markup changed; inspect the official table manually')
    for match in rows:
        print(match.group(1), re.findall(r'`([^`]+ / MTok)`', match.group(2)))
PY
```

3. Verify the table headings before interpreting columns. On this audit date,
   K3 columns are input, output, cache read, write 5m, write 1h; K2 has the first
   three only. Keep raw snapshots temporary and commit concise source-backed
   conclusions. Update only provider-owned defaults; record implementation and
   regression evidence separately before claiming readiness.
