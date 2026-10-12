"""Reproduce and qualify the scoped Zhipu catalogue update on its own branch."""

import base64
import importlib.util
import json
import os
from pathlib import Path
import subprocess

BASE = "e2d0eda26aea89c374a1f5cce38c439380a8b917"
BRANCH = "fix/zhipu-model-metadata-20261012"
ROOT = "TestGLM53FamilyPublishedMetadata"
OUT = Path(os.environ["RUNNER_TEMP"]) / "zhipu-catalog-evidence"
OUT.mkdir(parents=True, exist_ok=True)
PACKAGES = ["./relay/adaptor/zhipu", "./relay/adaptor/zai", "./relay/pricing"]


def run(label, command, check=True):
    """run executes a command and saves complete stdout, stderr, and its receipt."""
    result = subprocess.run(command, capture_output=True, text=True, timeout=1500)
    (OUT / f"{label}.stdout").write_text(result.stdout)
    (OUT / f"{label}.stderr").write_text(result.stderr)
    (OUT / f"{label}.receipt.json").write_text(json.dumps({"command": command, "exit_code": result.returncode}, indent=2))
    print(f"{label}: exit={result.returncode}", flush=True)
    if check and result.returncode:
        print(result.stdout[-10000:])
        print(result.stderr[-10000:])
        raise RuntimeError(f"{label} failed; full evidence is retained")
    return result


def replace_once(text, old, new):
    """replace_once replaces an exact expected fragment or rejects source drift."""
    if text.count(old) != 1:
        raise RuntimeError(f"unexpected source count for {old[:80]!r}")
    return text.replace(old, new, 1)


def edit_entry(path, model, replacements):
    """edit_entry applies exact replacements only within the named model entry."""
    file = Path(path)
    text = file.read_text()
    marker = f'\t"{model}": '
    if text.count(marker) != 1:
        raise RuntimeError(f"ambiguous model entry {model}")
    start = text.index(marker)
    end = text.index('\n\t},', start) + len('\n\t},')
    entry = text[start:end]
    for old, new in replacements:
        entry = replace_once(entry, old, new)
    file.write_text(text[:start] + entry + text[end:])


if os.environ["GITHUB_REF"] != f"refs/heads/{BRANCH}":
    raise RuntimeError("unexpected branch")
if run("base", ["git", "rev-parse", "HEAD^"]).stdout.strip() != BASE:
    raise RuntimeError("qualification must start from the reviewed base plus tests")

red = run("red", ["go", "test", "-json", "-count=1", "-timeout=10m", "-run", f"^{ROOT}$", "./relay/adaptor/zhipu"], check=False)
red_events = [json.loads(line) for line in red.stdout.splitlines() if line.strip()]
expected = set()
for provider in ("bigmodel", "zai"):
    for model in ("glm-5.3", "glm-5.3-flash"):
        for field in ("sampling", "features", "weights"):
            expected.add(f"{ROOT}/{provider}/{model}/{field}")
    expected.add(f"{ROOT}/{provider}/glm-5.3-flash/efforts")
leaves = {event.get("Test") for event in red_events if event.get("Action") == "fail" and event.get("Test", "").count("/") == 3}
if red.returncode != 1 or leaves != expected:
    print(red.stdout[-10000:])
    print(red.stderr[-10000:])
    raise RuntimeError(f"unexpected reproduction: expected {sorted(expected)}, got {sorted(leaves)}")
if any(event.get("Action") in ("build-fail", "skip") for event in red_events):
    raise RuntimeError("a build failure or skipped test is not a reproduction")
print(f"Confirmed {len(expected)} expected metadata failures before the fix.", flush=True)

text_path = "relay/adaptor/zhipu/constants_text.go"
vision_path = "relay/adaptor/zhipu/constants_vision.go"
zai_path = "relay/adaptor/zai/constants.go"
features = '[]string{"tools", "json_mode", "web_search", "reasoning"}'
sampling = '[]string{"temperature", "top_p", "max_tokens", "stop", "reasoning_effort"}'
common = [
    ("SupportedFeatures:           reasoningChatFeatures(),", f"SupportedFeatures:           {features},"),
    ("SupportedSamplingParameters: chatSamplingParameters(),", f"SupportedSamplingParameters: {sampling},"),
]
edit_entry(text_path, "glm-5.3", common + [
    ('DefaultReasoningEffort:      "max",', 'DefaultReasoningEffort:      "max",\n\t\tHuggingFaceID:               "zai-org/GLM-5.3",'),
])
text = Path(text_path).read_text()
text = replace_once(text, '\t// GLM-5.3: same base model as GLM-5.2 with post-training improvements.', '\t// GLM-5.3: same base model as GLM-5.2 with post-training improvements.\n\t// Metadata audited 2026-10-12; see docs/zhipu-model-catalog.md.\n\t// JSON mode is documented; provider-enforced JSON Schema is not.')
text = replace_once(text, '// international list quotes the same tier ($1.4 / $0.26 / $4.4).', '// international list is independently priced at $1.4 / $0.26 / $4.4.')
Path(text_path).write_text(text)
edit_entry(vision_path, "glm-5.3-flash", common + [
    (f'SupportedSamplingParameters: {sampling},', f'SupportedSamplingParameters: {sampling},\n\t\tSupportedReasoningEfforts:   []string{{"low", "high", "max"}},\n\t\tDefaultReasoningEffort:      "max",\n\t\tHuggingFaceID:               "zai-org/GLM-5.3-Flash",'),
    ('128K max output; list pricing', '128K max output; always-on thinking with reasoning_effort low/high/max (default max); list pricing'),
    ('with a 50% launch promotion applied as a time-window discount through 2026-09-09 24:00 (UTC+8).', 'the historical 50% launch promotion expired after 2026-09-09 24:00 (UTC+8).'),
])
text = Path(vision_path).read_text()
text = replace_once(text, '\t// GLM-5.3-Flash: launched 2026-08-26 with a two-week 50% launch promotion', '\t// Metadata audited 2026-10-12; see docs/zhipu-model-catalog.md.\n\t// GLM-5.3-Flash: launched 2026-08-26 with a two-week 50% launch promotion')
Path(vision_path).write_text(text)
text = Path(zai_path).read_text()
text = replace_once(text, '128K max output, always-on thinking. List pricing', '128K max output, always-on thinking with reasoning_effort low/high/max (default max). List pricing')
text = replace_once(text, 'with a 50% launch promotion applied as a time-window discount until 2026-09-09 24:00 UTC+8.', 'the historical 50% launch promotion expired after 2026-09-09 24:00 UTC+8.')
Path(zai_path).write_text(text)

Path("docs/zhipu-model-catalog.md").write_text('''# Zhipu / Z.AI model catalogue audit

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
''')

changed = [text_path, vision_path, zai_path, "docs/zhipu-model-catalog.md"]
run("gofmt", ["gofmt", "-w", text_path, vision_path, zai_path])
run("diff-check", ["git", "diff", "--check"])
run("module-unchanged", ["git", "diff", "--exit-code", "--", "go.mod", "go.sum"])

spec = importlib.util.spec_from_file_location("quick_tests", ".github/scripts/quick_tests.py")
quick = importlib.util.module_from_spec(spec)
spec.loader.exec_module(quick)
selections = {package.removeprefix("./"): None for package in PACKAGES}
discovery = run("discovery", ["go", "test", "-json", "-list", "^Test", *PACKAGES])
selected, inventory = quick.discover(quick.parse_events(discovery.stdout), selections=selections)
green = run("green", ["go", "test", "-race", "-json", "-count=1", "-timeout=10m", *PACKAGES])
counts = quick.validate_execution(quick.parse_events(green.stdout), selected)
run("vet", ["go", "vet", *PACKAGES])
run("quick", ["make", "test-quick"])
run("final-diff-check", ["git", "diff", "--check"])
run("final-module-unchanged", ["git", "diff", "--exit-code", "--", "go.mod", "go.sum"])
run("patch", ["git", "diff", "--", text_path, vision_path, zai_path])

summary = {"base": BASE, "reproduction_leaf_failures": len(expected), "passed_top_level_tests": counts, "scope": PACKAGES, "live_provider_calls": False}
(OUT / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2), flush=True)

# The token is used only to publish reviewed source edits to this task's branch.
# No force push, workflow mutation, deployment or main-branch write is permitted.
run("remote-head", ["git", "fetch", "origin", f"refs/heads/{BRANCH}"])
remote = run("remote-sha", ["git", "rev-parse", "FETCH_HEAD"]).stdout.strip()
if remote != os.environ["GITHUB_SHA"]:
    raise RuntimeError("branch changed during qualification; refusing to overwrite it")
run("author", ["git", "config", "user.name", "github-actions[bot]"])
run("author-email", ["git", "config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"])
run("stage", ["git", "add", "--", *changed])
run("commit", ["git", "commit", "-m", "fix(zhipu): refresh verified GLM-5.3 model metadata"])
head = run("qualified-sha", ["git", "rev-parse", "HEAD"]).stdout.strip()
auth = base64.b64encode(f"x-access-token:{os.environ.pop('GH_TOKEN')}".encode()).decode()
push_env = dict(os.environ, GIT_CONFIG_COUNT="1", GIT_CONFIG_KEY_0="http.https://github.com/.extraheader", GIT_CONFIG_VALUE_0=f"AUTHORIZATION: basic {auth}")
subprocess.run(["git", "push", "origin", f"HEAD:refs/heads/{BRANCH}"], env=push_env, check=True, timeout=120)
summary["qualified_commit"] = head
(OUT / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2), flush=True)
